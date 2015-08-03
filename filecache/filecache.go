package filecache

import "sync"

type Cache struct {
	size, maxSize int64
	files         map[string]*Entry
	keys          []string
	lock          sync.Mutex
}

type Entry struct {
	Filename, MimeType string
	Bytes              []byte
}

func (e *Entry) size() int64 {
	return int64(len(e.Bytes))
}

func New(s int64) *Cache {
	if s <= 0 {
		panic("must be larger than 0")
	}
	return &Cache{
		maxSize: s,
		keys:    []string{},
		files:   map[string]*Entry{},
	}
}

func (c *Cache) Get(key string) *Entry {
	c.lock.Lock()
	defer c.lock.Unlock()
	e, ok := c.files[key]
	if !ok {
		return nil
	}
	return e
}

func (c *Cache) Add(key, f, m string, b []byte) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	e, ok := c.files[key]
	if ok {
		c.size -= e.size()
	} else {
		c.keys = append(c.keys, key)
	}
	e = &Entry{f, m, b}
	c.size += e.size()
	for c.size > c.maxSize && len(c.keys) > 0 {
		rk := c.keys[0]
		c.size -= c.files[rk].size()
		delete(c.files, rk)
		c.keys = c.keys[1:]
	}
	c.files[key] = e
	return !ok
}

func (c *Cache) Size() int64 {
	return c.size
}

func (c *Cache) Keys() []string {
	return c.keys
}
