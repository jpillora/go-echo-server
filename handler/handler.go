package echo

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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

	"golang.org/x/net/websocket"

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
	ws       http.Handler
}

type echoStats struct {
	Uptime time.Time
	Echoes int
}

func New(c Config) http.Handler {
	e := &echoHandler{
		config: c,
		cache:  filecache.New(250 * 1000 * 1000), //250Mb
		stats: echoStats{
			Uptime: time.Now(),
		},
		requests: nil,
	}
	e.ws = websocket.Handler(e.serveWS)
	var h http.Handler = e
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
	Path     string            `json:"path,omitempty"`
	Headers  map[string]string `json:"headers"`
	Form     map[string]body   `json:"form,omitempty"`
	Body     body              `json:"body,omitempty"`
	Error    error             `json:"error,omitempty"`
	Sleep    string            `json:"sleep,omitempty"`
	Status   int               `json:"status,omitempty"`
}

var (
	filePath     = regexp.MustCompile(`^\/file\/([a-f0-9]{` + strconv.Itoa(md5.Size*2) + `})$`)
	delayPath    = regexp.MustCompile(`\/(sleep|delay)\/([0-9]+)(m?s)?(\/|$)?`)
	statusPath   = regexp.MustCompile(`\/status\/([0-9]{3})(\/|$)`)
	authPath     = regexp.MustCompile(`\/auth\/(\w+):(\w+)(\/|$)`)
	echoPath     = regexp.MustCompile(`^\/echo(es)?(\/([0-9]+))?$`)
	cityPath     = regexp.MustCompile(`(-[A-Z]+)$`)
	formType     = regexp.MustCompile(`\bform\b`)
	defaultmtype = "application/octet-stream"
)

func (e *echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	h := r.Header
	//echo request
	req := request{
		Time:     time.Now(),
		Location: h.Get("cf-ipcountry"),
		Method:   r.Method,
		Host:     r.Host,
		Path:     r.URL.RequestURI(),
		Headers:  map[string]string{},
	}

	if m := authPath.FindStringSubmatch(req.Path); len(m) > 0 {
		if u, p, ok := r.BasicAuth(); !ok || u != m[1] || p != m[2] {
			w.Header().Set("WWW-Authenticate", "Basic")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}
	}

	//special echo-websockets mode
	if h.Get("Connection") == "Upgrade" {
		e.ws.ServeHTTP(w, r)
		return
	}

	//special cases
	if req.Path == "/favicon.ico" {
		return
	} else if req.Path == "/ping" {
		w.Write([]byte("pong"))
		return
	} else if r.URL.Path == "/proxy.html" {
		//xdomain cors proxy
		src := r.URL.Query().Get("src")
		if src == "" {
			src = "//cdn.rawgit.com/jpillora/xdomain/0.7.3/dist/xdomain.min.js"
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
		<!DOCTYPE HTML>
		<script src="` + src + `" master="http://abc.example.com"></script>
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
	e.requests = append(e.requests, &req)
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
	for k, _ := range h {
		k = strings.ToLower(k)
		v := h.Get(k)
		if strings.HasPrefix(k, "cf-") || strings.HasPrefix(k, "x-") {
			if debug {
				fmt.Printf("%d: skipping header %s=%s", id, k, v)
			}
			continue
		}
		req.Headers[k] = v
	}

	//parse form fields
	contentType := h.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data;") {
		req.Form = map[string]body{}
		if reader, err := r.MultipartReader(); err != nil {
			req.Error = err
		} else {
			for {
				p, err := reader.NextPart()
				if err != nil {
					if err != io.EOF {
						req.Error = err
					}
					break
				}
				k := p.FormName()
				body, err := e.extractBody(p, p.FileName(), p.Header.Get("Content-Type"))
				if err != nil {
					req.Error = err
					break
				}
				req.Form[k] = body
			}
		}
	} else if formType.MatchString(contentType) {
		req.Form = map[string]body{}
		if err := r.ParseForm(); err != nil {
			req.Error = err
		} else {
			for k, _ := range r.Form {
				if p, h, err := r.FormFile(k); err == nil {
					body, err := e.extractBody(p, h.Filename, h.Header.Get("Content-Type"))
					if err != nil {
						fmt.Printf("Failed to extract form file: %s: %s", k, err)
					} else {
						req.Form[k] = body
					}
				} else {
					req.Form[k] = body(r.FormValue(k))
				}
			}
		}
	} else {
		//otherwise just use body as is
		body, err := e.extractBody(r.Body, r.URL.Path, h.Get("Content-Type"))
		if err != nil {
			fmt.Printf("Failed to extract body: %s", err)
			req.Error = err
		} else {
			req.Body = body
		}
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
	b, _ := json.MarshalIndent(&req, "", "  ")
	w.Header().Set("Content-Type", jsonType)
	w.WriteHeader(status)
	w.Write(b)
}

func (e *echoHandler) serveWS(conn *websocket.Conn) {
	c := conn.Config()
	r := conn.Request()
	h := map[string]string{}
	for k, _ := range r.Header {
		h[k] = r.Header.Get(k)
	}
	b, _ := json.MarshalIndent(&struct {
		Location string
		Origin   string
		Protocol []string
		Version  int
		Headers  map[string]string
	}{
		Location: c.Location.String(),
		Origin:   c.Origin.String(),
		Protocol: c.Protocol,
		Version:  c.Version,
		Headers:  h,
	}, "", "  ")
	conn.Write(b)
	io.Copy(conn, conn)
	conn.Close()
}

type body interface{}

type bodyValues struct {
	String string `json:"val,omitempty"`
	Length int    `json:"length,omitempty"`
	Type   string `json:"type,omitempty"`
	MD5    string `json:"md5,omitempty"`
	URL    string `json:"url,omitempty"`
}

func (e *echoHandler) extractBody(r io.ReadCloser, fileName, mimeType string) (body, error) {
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	//either display it as a string or store as file
	if utf8.Valid(bytes) {
		return body(string(bytes)), nil
	}

	//calc mime
	if (mimeType == "" || mimeType == defaultmtype) && fileName != "" {
		mimeType = mime.TypeByExtension(filepath.Ext(fileName))
	}
	//hash bytes
	hash := md5.New()
	hash.Write([]byte(mimeType + "|"))
	hash.Write(bytes)
	md5 := hex.EncodeToString(hash.Sum(nil))

	b := &bodyValues{
		Length: len(bytes),
		Type:   mimeType,
		MD5:    md5,
		URL:    "/file/" + md5,
	}
	e.cache.Add(b.MD5, fileName, mimeType, bytes)
	return b, nil
}
