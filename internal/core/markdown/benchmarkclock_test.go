package markdown

import "time"

var benchmarkNow = func() int64 { return time.Now().UnixNano() }
