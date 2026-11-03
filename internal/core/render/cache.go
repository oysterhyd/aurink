package render

import (
	"container/list"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"aurink/internal/contract"
)

const memoryBytes = 32 << 20
const diskBytes = 64 << 20

type cacheEntry struct {
	key    [32]byte
	result contract.RenderResult
	size   int
}
type cache struct {
	mu    sync.Mutex
	dir   string
	items map[[32]byte]*list.Element
	lru   list.List
	bytes int
}

func newCache(dir string) *cache { return &cache{dir: dir, items: make(map[[32]byte]*list.Element)} }

func (c *cache) get(key [32]byte, kind contract.RenderKind) (contract.RenderResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.items[key]; e != nil {
		c.lru.MoveToFront(e)
		return cloneResult(e.Value.(cacheEntry).result), true
	}
	if c.dir == "" {
		return contract.RenderResult{}, false
	}
	path := filepath.Join(c.dir, hex.EncodeToString(key[:])+".json")
	info, err := os.Stat(path)
	if err != nil || info.Size() > diskBytes {
		return contract.RenderResult{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > diskBytes {
		return contract.RenderResult{}, false
	}
	result, err := parseAnswer(data, kind)
	if err != nil {
		return contract.RenderResult{}, false
	}
	c.putMemory(key, result)
	return cloneResult(result), true
}

func (c *cache) put(key [32]byte, result contract.RenderResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putMemory(key, result)
	if c.dir == "" {
		return
	}
	data, err := json.Marshal(pageAnswer{SVG: string(result.SVG), PNG: result.PNG, Width: result.Width, Height: result.Height, Ascent: result.Ascent, Scale: result.Scale})
	if err != nil || len(data) > diskBytes || os.MkdirAll(c.dir, 0755) != nil {
		return
	}
	f, err := os.CreateTemp(c.dir, ".render-*")
	if err != nil {
		return
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return
	}
	if err = f.Close(); err != nil {
		return
	}
	if err = os.Rename(name, filepath.Join(c.dir, hex.EncodeToString(key[:])+".json")); err != nil {
		return
	}
	c.evictDisk()
}

func (c *cache) putMemory(key [32]byte, result contract.RenderResult) {
	if e := c.items[key]; e != nil {
		c.bytes -= e.Value.(cacheEntry).size
		c.lru.Remove(e)
		delete(c.items, key)
	}
	size := len(result.SVG) + len(result.PNG)
	if size > memoryBytes {
		return
	}
	e := c.lru.PushFront(cacheEntry{key, cloneResult(result), size})
	c.items[key] = e
	c.bytes += size
	for c.lru.Len() > 256 || c.bytes > memoryBytes {
		old := c.lru.Back()
		entry := old.Value.(cacheEntry)
		c.bytes -= entry.size
		delete(c.items, entry.key)
		c.lru.Remove(old)
	}
}

func (c *cache) evictDisk() {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type diskEntry struct {
		name     string
		size     int64
		modified int64
	}
	var files []diskEntry
	var total int64
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" || len(e.Name()) != 69 {
			continue
		}
		if _, err := hex.DecodeString(e.Name()[:64]); err != nil {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, diskEntry{e.Name(), info.Size(), info.ModTime().UnixNano()})
		total += info.Size()
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified < files[j].modified })
	for _, f := range files {
		if total <= diskBytes {
			break
		}
		if os.Remove(filepath.Join(c.dir, f.name)) == nil {
			total -= f.size
		}
	}
}
