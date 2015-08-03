package echo

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"mime"
	"net"
	"net/http"
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
	Time                          time.Time
	Duration                      string
	IP, Proto, Host, Method, Path string
	Headers                       map[string]string
	Body, BodyURL, BodyMD5        string `json:",omitempty"`
	Error, Sleep                  string `json:",omitempty"`
	Status                        int    `json:",omitempty"`
}

var (
	filePath     = regexp.MustCompile(`^\/file\/([a-f0-9]{` + strconv.Itoa(md5.Size*2) + `})$`)
	delayPath    = regexp.MustCompile(`\/(sleep|delay)\/([0-9]+)(m?s)?(\/$)?`)
	statusPath   = regexp.MustCompile(`\/status\/([0-9]{3})(\/$)?`)
	echoPath     = regexp.MustCompile(`^\/echo(es)?(\/([0-9]+))?$`)
	defaultmtype = "application/octet-stream"
)

func (e *echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	//echo request
	req := &request{
		Time:    time.Now(),
		Method:  r.Method,
		Host:    r.Host,
		Path:    r.URL.RequestURI(),
		Headers: map[string]string{},
	}

	//get ip address
	h := r.Header
	req.IP = h.Get("CF-Connecting-IP")
	if req.IP == "" {
		req.IP = h.Get("X-Forwarded-For")
	}
	if req.IP == "" {
		req.IP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	req.Proto = h.Get("X-Forwarded-Proto")
	if req.Proto == "" {
		if r.TLS == nil {
			req.Proto = "http"
		} else {
			req.Proto = "https"
		}
	}

	e.lock.Lock()
	if m := echoPath.FindStringSubmatch(req.Path); len(m) > 0 {
		var v interface{}
		if i, err := strconv.Atoi(m[3]); err == nil && i < len(e.requests) {
			v = e.requests[i]
		} else {
			v = &e.stats
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		e.lock.Unlock()
		return
	}
	e.requests = append(e.requests, req)
	e.stats.Echoes++
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

	size := bytes.MinRead
	for k, _ := range h {
		k = strings.ToLower(k)
		v := h.Get(k)
		if k == "content-length" {
			if n, err := strconv.Atoi(v); err == nil {
				size = n
			}
		} /*else if strings.HasPrefix(k, "cf-") {
			continue
		}*/
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b)
}
