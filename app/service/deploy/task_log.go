package deploy

import (
	"io"
	"sync"
)

type TaskLog struct {
	mux    sync.RWMutex
	buf    []byte
	isOver bool
}

func (tl *TaskLog) Write(b []byte) (n int, err error) {
	tl.mux.Lock()
	defer tl.mux.Unlock()
	tl.buf = append(tl.buf, b...)
	return len(b), nil
}

func (tl *TaskLog) ReadAt(b []byte, off int) (n int, err error) {
	tl.mux.RLock()
	defer tl.mux.RUnlock()
	l := len(tl.buf)
	if l > off {
		n = copy(b, tl.buf[off:l])
	}
	if off+n >= l && tl.isOver {
		err = io.EOF
	}
	return
}

func (tl *TaskLog) WriteOver() {
	tl.mux.Lock()
	defer tl.mux.Unlock()
	tl.isOver = true
}
