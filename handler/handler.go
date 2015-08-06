package echo

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jpillora/go-echo-server/filecache"
	"github.com/jpillora/requestlog"
)

var (
	jsonType = "application/json; charset=utf8"
	debug    = os.Getenv("DEBUG") != ""
)

type Config struct {
	Log bool `help:"Enable logging"`
	// Verbose bool `help:"Enable verbose logging"`
}

type echoHandler struct {
	config Config
	cache  *filecache.Cache
	//metrics
	lock     sync.Mutex
	stats    echoStats
	requests []*request
}

type echoStats struct {
	Uptime time.Time
	Echoes int
}

func New(c Config) http.Handler {
	var h http.Handler = &echoHandler{
		config: c,
		cache:  filecache.New(250 * 1000 * 1000), //250Mb
		stats: echoStats{
			Uptime: time.Now(),
		},
		requests: nil,
	}
	if c.Log {
		h = requestlog.Wrap(h)
	}
	return h
}

type request struct {
	Time     time.Time         `json:"time"`
	Duration string            `json:"duration"`
	Location string            `json:"location,omitempty"`
	IP       string            `json:"ip"`
	DNS      string            `json:"dns,omitempty"`
	Proto    string            `json:"proto,omitempty"`
	Host     string            `json:"host,omitempty"`
	Method   string            `json:"method,omitempty"`
	Path     string            `json:",omitempty"`
	Headers  map[string]string `json:"headers"`
	Body     string            `json:"body,omitempty"`
	BodyURL  string            `json:"bodyURL,omitempty"`
	BodyMD5  string            `json:"bodyMD5,omitempty"`
	Error    string            `json:"error,omitempty"`
	Sleep    string            `json:"sleep,omitempty"`
	Status   int               `json:"status,omitempty"`
}

var (
	filePath     = regexp.MustCompile(`^\/file\/([a-f0-9]{` + strconv.Itoa(md5.Size*2) + `})$`)
	delayPath    = regexp.MustCompile(`\/(sleep|delay)\/([0-9]+)(m?s)?(\/$)?`)
	statusPath   = regexp.MustCompile(`\/status\/([0-9]{3})(\/$)?`)
	echoPath     = regexp.MustCompile(`^\/echo(es)?(\/([0-9]+))?$`)
	cityPath     = regexp.MustCompile(`(-[A-Z]+)$`)
	defaultmtype = "application/octet-stream"
)

func (e *echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := r.Header

	//echo request
	req := &request{
		Time:     time.Now(),
		Location: h.Get("cf-ipcountry"),
		Method:   r.Method,
		Host:     r.Host,
		Path:     r.URL.RequestURI(),
		Headers:  map[string]string{},
	}

	if req.Path == "/ping" {
		w.Write([]byte("pong"))
		return
	}

	if req.Path == "/proxy.html" {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
		<!DOCTYPE HTML>
		<script src="//cdn.rawgit.com/jpillora/xdomain/0.7.3/dist/xdomain.min.js" master="http://abc.example.com"></script>
		`))
		return
	}

	//cors
	if o := h.Get("Origin"); o != req.Host {
		w.Header().Set("Access-Control-Allow-Origin", o)
		w.Header().Set("Access-Control-Allow-Methods", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")
	}

	//extract city from cloudflare ray ID
	if m := cityPath.FindStringSubmatch(h.Get("cf-ray")); len(m) > 0 {
		req.Location += m[1]
	}

	//get ip address
	req.IP = h.Get("CF-Connecting-IP")
	if req.IP == "" {
		req.IP = h.Get("X-Forwarded-For")
	}
	if req.IP == "" {
		req.IP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	if hosts, err := net.LookupAddr(req.IP); err == nil && len(hosts) > 0 {
		req.DNS = hosts[0]
	}

	req.Proto = h.Get("X-Forwarded-Proto")
	if req.Proto == "" {
		if r.TLS == nil {
			req.Proto = "http"
		} else {
			req.Proto = "https"
		}
	}

	//handle stats
	e.lock.Lock()
	if m := echoPath.FindStringSubmatch(req.Path); len(m) > 0 {
		var v interface{}
		if i, err := strconv.Atoi(m[3]); err == nil && i < len(e.requests) {
			v = e.requests[i]
		} else {
			v = &e.stats
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		w.Header().Set("Content-Type", jsonType)
		w.Write(b)
		e.lock.Unlock()
		return
	}
	e.requests = append(e.requests, req)
	e.stats.Echoes++
	id := e.stats.Echoes
	e.lock.Unlock()

	//return files
	if m := filePath.FindStringSubmatch(req.Path); len(m) > 0 {
		r.Body.Close()
		entry := e.cache.Get(m[1])
		if entry == nil {
			w.WriteHeader(404)
			w.Write([]byte("File not found"))
			return
		}
		if entry.MimeType == "" {
			entry.MimeType = defaultmtype
		}
		w.Header().Set("Content-Type", entry.MimeType)
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.Bytes)))
		w.Write(entry.Bytes)
		return
	}

	//copy headers
	size := bytes.MinRead
	for k, _ := range h {
		k = strings.ToLower(k)
		v := h.Get(k)
		if k == "content-length" {
			if n, err := strconv.Atoi(v); err == nil {
				size = n
			}
		} else if strings.HasPrefix(k, "cf-") || strings.HasPrefix(k, "x-") {
			if debug {
				fmt.Printf("%d: skipping header %s=%s", id, k, v)
			}
			continue
		}
		req.Headers[k] = v
	}

	fname := ""
	mtype := h.Get("Content-Type")
	body := r.Body
	//extract file from multipart form
	if r, err := r.MultipartReader(); err == nil {
		p, err := r.NextPart()
		if err == nil && p.FormName() == "file" {
			fname = p.FileName()
			mtype = p.Header.Get("Content-Type")
			if mtype == defaultmtype {
				mtype = mime.TypeByExtension(filepath.Ext(p.FileName()))
			}
			body = p
		}
	}

	defer body.Close()
	buf := bytes.NewBuffer(make([]byte, 0, size))
	n, err := buf.ReadFrom(body)
	if err == nil && n > 0 {
		b := buf.Bytes()
		hash := md5.New()
		hash.Write([]byte(mtype + "|"))
		hash.Write(b)
		req.BodyMD5 = hex.EncodeToString(hash.Sum(nil))
		if utf8.Valid(b) {
			req.Body = string(b)
		} else {
			e.cache.Add(req.BodyMD5, fname, mtype, b)
			req.BodyURL = req.Proto + "://" + req.Host + "/file/" + req.BodyMD5
		}
	} else if err != nil {
		req.Error = "Download failed: " + err.Error()
	}

	if m := delayPath.FindStringSubmatch(req.Path); len(m) > 0 {
		unit := m[3]
		if unit == "" {
			unit = "ms"
		}
		if d, err := time.ParseDuration(m[2] + unit); err == nil && d < time.Minute {
			req.Sleep = d.String()
			time.Sleep(d)
		}
	}

	status := 200
	if m := statusPath.FindStringSubmatch(req.Path); len(m) > 0 {
		if code, err := strconv.Atoi(m[1]); err == nil {
			req.Status = code
			status = code
		}
	}

	req.Duration = time.Since(req.Time).String()
	b, _ := json.MarshalIndent(req, "", "  ")
	w.Header().Set("Content-Type", jsonType)
	w.WriteHeader(status)
	w.Write(b)
}
