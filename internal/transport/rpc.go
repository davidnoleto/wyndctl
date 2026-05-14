package transport

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Method describes a single RPC method on the Sentry service.
type Method struct {
	PackageID       uint64
	ServiceID       uint64
	MethodID        uint64
	ServerStreaming  bool
	ClientStreaming  bool
}

// Call represents a decoded RPC invocation header.
type Call struct {
	PackageID  uint64
	ServiceID  uint64
	MethodID   uint64
	Invocation uint64
}

// ClientContext tracks the state of an outgoing RPC call.
type ClientContext struct {
	Channel    *SerialChannel
	Invocation uint64
	Method     *Method
	ResponseCh chan []byte
}

// RPC manages the RPC protocol on top of the Transport layer.
type RPC struct {
	transport      *Transport
	nextInvocation atomic.Uint64
	clientContexts []*ClientContext
	mu             sync.Mutex
	UnaryTimeout   time.Duration
}

// NewRPC creates a new RPC instance wired to the given transport.
func NewRPC(t *Transport) *RPC {
	rpc := &RPC{
		transport:    t,
		UnaryTimeout: 2 * time.Second,
	}

	t.SetHandler(PacketClientResponse, rpc.handleClientResponse)
	t.SetHandler(PacketClientFinalize, rpc.handleClientFinalize)

	return rpc
}

// UnaryCall sends a request and waits for a single response.
func (r *RPC) UnaryCall(method *Method, request []byte, channel *SerialChannel, timeout time.Duration) ([]byte, error) {
	if timeout == 0 {
		timeout = r.UnaryTimeout
	}

	ctx := r.allocateClientContext(channel, method)
	defer r.freeClientContext(ctx)

	if err := r.sendServerRequest(ctx, request); err != nil {
		return nil, fmt.Errorf("rpc: sending request: %w", err)
	}

	select {
	case resp := <-ctx.ResponseCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, &TimeoutError{Method: method, Timeout: timeout}
	}
}

// StreamingCall initiates a bidirectional streaming RPC (used for firmware writes).
func (r *RPC) StreamingCall(method *Method, channel *SerialChannel) *ClientContext {
	ctx := r.allocateClientContext(channel, method)
	r.sendPacket(ctx, PacketServerInitialize, nil)
	return ctx
}

// StreamingSend sends a request on an open streaming call.
func (r *RPC) StreamingSend(ctx *ClientContext, request []byte) error {
	return r.sendServerRequest(ctx, request)
}

// StreamingReceive waits for the next response on a streaming call.
func (r *RPC) StreamingReceive(ctx *ClientContext, timeout time.Duration) ([]byte, error) {
	select {
	case resp := <-ctx.ResponseCh:
		return resp, nil
	case <-time.After(timeout):
		return nil, &TimeoutError{Method: ctx.Method, Timeout: timeout}
	}
}

// StreamingFinalize closes a streaming call.
func (r *RPC) StreamingFinalize(ctx *ClientContext) {
	r.sendPacket(ctx, PacketServerFinalize, nil)
	r.freeClientContext(ctx)
}

func (r *RPC) allocateClientContext(channel *SerialChannel, method *Method) *ClientContext {
	ctx := &ClientContext{
		Channel:    channel,
		Invocation: r.nextInvocation.Add(1),
		Method:     method,
		ResponseCh: make(chan []byte, 8),
	}

	r.mu.Lock()
	r.clientContexts = append(r.clientContexts, ctx)
	r.mu.Unlock()

	return ctx
}

func (r *RPC) freeClientContext(ctx *ClientContext) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, c := range r.clientContexts {
		if c == ctx {
			r.clientContexts = append(r.clientContexts[:i], r.clientContexts[i+1:]...)
			return
		}
	}
}

func (r *RPC) findClientContext(channel *SerialChannel, call *Call) *ClientContext {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ctx := range r.clientContexts {
		if ctx.Channel != channel {
			continue
		}
		if ctx.Method.PackageID != call.PackageID ||
			ctx.Method.ServiceID != call.ServiceID ||
			ctx.Method.MethodID != call.MethodID {
			continue
		}
		if ctx.Invocation != call.Invocation {
			continue
		}
		return ctx
	}
	return nil
}

func (r *RPC) sendServerRequest(ctx *ClientContext, request []byte) error {
	return r.sendPacket(ctx, PacketServerRequest, request)
}

func (r *RPC) sendPacket(ctx *ClientContext, pType PacketType, message []byte) error {
	data := encodeCallHeader(ctx.Method, ctx.Invocation)
	if message != nil {
		data = append(data, message...)
	}
	return r.transport.Write(ctx.Channel, pType, data)
}

func (r *RPC) handleClientResponse(channel *SerialChannel, data []byte) {
	call, pos, err := decodeCall(data, 0)
	if err != nil {
		return
	}

	ctx := r.findClientContext(channel, call)
	if ctx == nil {
		return
	}

	response := make([]byte, len(data[pos:]))
	copy(response, data[pos:])

	select {
	case ctx.ResponseCh <- response:
	default:
	}
}

func (r *RPC) handleClientFinalize(channel *SerialChannel, data []byte) {
	call, _, err := decodeCall(data, 0)
	if err != nil {
		return
	}

	ctx := r.findClientContext(channel, call)
	if ctx == nil {
		return
	}

	close(ctx.ResponseCh)
}

func encodeCallHeader(method *Method, invocation uint64) []byte {
	var data []byte
	data = EncodeVarint(data, method.PackageID)
	data = EncodeVarint(data, method.ServiceID)
	data = EncodeVarint(data, method.MethodID)
	data = EncodeVarint(data, invocation)
	return data
}

func decodeCall(data []byte, pos int) (*Call, int, error) {
	packageID, pos, err := DecodeVarint(data, pos)
	if err != nil {
		return nil, pos, fmt.Errorf("decoding package_id: %w", err)
	}

	serviceID, pos, err := DecodeVarint(data, pos)
	if err != nil {
		return nil, pos, fmt.Errorf("decoding service_id: %w", err)
	}

	methodID, pos, err := DecodeVarint(data, pos)
	if err != nil {
		return nil, pos, fmt.Errorf("decoding method_id: %w", err)
	}

	invocation, pos, err := DecodeVarint(data, pos)
	if err != nil {
		return nil, pos, fmt.Errorf("decoding invocation: %w", err)
	}

	return &Call{
		PackageID:  packageID,
		ServiceID:  serviceID,
		MethodID:   methodID,
		Invocation: invocation,
	}, pos, nil
}

// TimeoutError indicates an RPC call exceeded the deadline.
type TimeoutError struct {
	Method  *Method
	Timeout time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("rpc: call timed out after %s (method %d/%d/%d)",
		e.Timeout, e.Method.PackageID, e.Method.ServiceID, e.Method.MethodID)
}
