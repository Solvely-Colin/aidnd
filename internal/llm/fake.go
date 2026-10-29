package llm

import (
	"context"
	"sync"
)

// Reply produces a scripted response for a request.
type Reply func(req Request) (Response, error)

// Text returns a fixed text reply.
func Text(s string) Reply {
	return func(Request) (Response, error) { return Response{Content: s}, nil }
}

// Calls returns a reply made of tool calls.
func Calls(calls ...ToolCall) Reply {
	return func(Request) (Response, error) { return Response{ToolCalls: calls}, nil }
}

// Fake is a scripted Client. Replies are queued per request Tag and used
// in order; when a tag's queue is empty the Default for that tag (or the
// global Fallback) answers. Every request is recorded.
type Fake struct {
	mu       sync.Mutex
	queues   map[string][]Reply
	Defaults map[string]Reply
	Fallback Reply
	Requests []Request
}

// NewFake returns an empty fake whose fallback is a short text.
func NewFake() *Fake {
	return &Fake{queues: map[string][]Reply{}, Defaults: map[string]Reply{}, Fallback: Text("The moment passes.")}
}

// On queues replies for a tag.
func (f *Fake) On(tag string, replies ...Reply) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queues[tag] = append(f.queues[tag], replies...)
	return f
}

// Default sets the reply used when a tag's queue is empty.
func (f *Fake) Default(tag string, r Reply) *Fake {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Defaults[tag] = r
	return f
}

// Chat implements Client.
func (f *Fake) Chat(ctx context.Context, req Request) (Response, error) {
	f.mu.Lock()
	f.Requests = append(f.Requests, req)
	var r Reply
	if q := f.queues[req.Tag]; len(q) > 0 {
		r, f.queues[req.Tag] = q[0], q[1:]
	} else if d, ok := f.Defaults[req.Tag]; ok {
		r = d
	} else {
		r = f.Fallback
	}
	f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	return r(req)
}

// Pending reports how many queued replies remain for tag.
func (f *Fake) Pending(tag string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.queues[tag])
}

// Tagged returns the recorded requests with the given tag.
func (f *Fake) Tagged(tag string) []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Request
	for _, r := range f.Requests {
		if r.Tag == tag {
			out = append(out, r)
		}
	}
	return out
}
