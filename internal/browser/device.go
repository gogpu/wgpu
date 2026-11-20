//go:build js && wasm

package browser

import (
	"syscall/js"

	"github.com/gogpu/wgpu/internal/loststate"
)

// Device wraps a browser GPUDevice with pre-bound creation methods.
//
// Pre-binding JS methods at construction time avoids repeated .Get("methodName")
// calls on every frame. This pattern is used by Ebiten and other Go WASM libraries
// for optimal performance.
//
// Matches Rust wgpu WebDevice which holds the webgpu_sys::GpuDevice value.
type Device struct {
	// ref_ is the GPUDevice JavaScript object.
	ref_ js.Value

	// queue is the device's command queue (GPUQueue).
	queue *Queue

	// features is the device's GPUSupportedFeatures.
	features js.Value

	// limits is the device's GPUSupportedLimits.
	limits js.Value

	// Pre-bound creation methods. Each is the result of
	// device.methodName.bind(device), so calling fnX.Invoke(args...)
	// is equivalent to device.methodName(args...) but without the
	// property lookup overhead on each call.
	fnCreateBuffer          js.Value
	fnCreateTexture         js.Value
	fnCreateShaderModule    js.Value
	fnCreateBindGroupLayout js.Value
	fnCreateBindGroup       js.Value
	fnCreatePipelineLayout  js.Value
	fnCreateRenderPipeline  js.Value
	fnCreateComputePipeline js.Value
	fnCreateCommandEncoder  js.Value
	fnCreateSampler         js.Value
	fnCreateQuerySet        js.Value

	// lost is the device-lost and uncaptured-error registration state machine
	// (P1-API-042 / #417). NewDevice creates it and attaches the JS handlers
	// that record into it; the root declaration layer registers callbacks over
	// the seam methods below.
	lost *loststate.State[*GPUErrorInfo]

	// fnLostResolve and fnUncaptured hold the JS callbacks the handlers are
	// built from. The promise and the event target keep their own JS
	// references, but the Go side must keep the js.Func alive for as long as
	// the device can receive the event.
	fnLostResolve js.Func
	fnUncaptured  js.Func
}

// NewDevice constructs a Device from a GPUDevice js.Value.
// Pre-binds all creation methods and extracts the queue.
func NewDevice(ref js.Value) *Device {
	d := &Device{
		ref_:     ref,
		features: ref.Get("features"),
		limits:   ref.Get("limits"),
		lost:     loststate.New[*GPUErrorInfo](),
	}

	// Extract the device's queue (GPUDevice.queue is a readonly attribute).
	d.queue = NewQueue(ref.Get("queue"))

	// Pre-bind all creation methods to avoid property lookups on hot paths.
	// pattern: device.createX.bind(device) => fnCreateX
	d.fnCreateBuffer = bindMethod(ref, "createBuffer")
	d.fnCreateTexture = bindMethod(ref, "createTexture")
	d.fnCreateShaderModule = bindMethod(ref, "createShaderModule")
	d.fnCreateBindGroupLayout = bindMethod(ref, "createBindGroupLayout")
	d.fnCreateBindGroup = bindMethod(ref, "createBindGroup")
	d.fnCreatePipelineLayout = bindMethod(ref, "createPipelineLayout")
	d.fnCreateRenderPipeline = bindMethod(ref, "createRenderPipeline")
	d.fnCreateComputePipeline = bindMethod(ref, "createComputePipeline")
	d.fnCreateCommandEncoder = bindMethod(ref, "createCommandEncoder")
	d.fnCreateSampler = bindMethod(ref, "createSampler")
	d.fnCreateQuerySet = bindMethod(ref, "createQuerySet")

	d.attachLostHandlers()

	return d
}

// attachLostHandlers wires GPUDevice.lost and the uncapturederror event to the
// device's loststate. It runs once at construction, on the single JS thread, so
// every recording and dispatch path below also runs there.
func (d *Device) attachLostHandlers() {
	if d.ref_.IsUndefined() || d.ref_.IsNull() {
		return
	}

	// GPUDevice.lost is a promise that resolves with a GPUDeviceLostInfo when
	// the device is lost and never rejects (WebGPU spec, GPUDevice.lost).
	if lost := d.ref_.Get("lost"); !lost.IsUndefined() && !lost.IsNull() {
		d.fnLostResolve = js.FuncOf(d.onDeviceLost)
		lost.Call("then", d.fnLostResolve)
	}

	// The uncapturederror event carries every error that reached no error
	// scope. Assigning the onuncapturederror handler property is the spec's
	// event-handler attribute, the wiring this package uses for JS callbacks.
	d.fnUncaptured = js.FuncOf(d.onUncapturedError)
	d.ref_.Set("onuncapturederror", d.fnUncaptured)
}

// onDeviceLost is the GPUDevice.lost promise reaction. It latches the loss once:
// loststate's first-wins rule keeps a driver loss recorded earlier and makes a
// later Release/destroy reaction a no-op.
func (d *Device) onDeviceLost(_ js.Value, args []js.Value) any {
	var reason, message string
	if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
		info := args[0]
		reason = jsStringProperty(info, "reason")
		message = jsStringProperty(info, "message")
	}
	d.markLost(lostReasonFromJS(reason), message)
	return nil
}

// onUncapturedError is the uncapturederror event handler. The event's error is
// one of the three GPUError classes; an unmapped class is dropped rather than
// invented into a category, as the pop path reports an error for an
// unrecognized class instead of guessing one.
func (d *Device) onUncapturedError(_ js.Value, args []js.Value) any {
	if len(args) == 0 || args[0].IsUndefined() || args[0].IsNull() {
		return nil
	}
	jsErr := args[0].Get("error")
	if jsErr.IsUndefined() || jsErr.IsNull() {
		return nil
	}
	filter, ok := ErrorNameToFilter(jsConstructorName(jsErr))
	if !ok {
		return nil
	}
	d.recordUncaptured(&GPUErrorInfo{Type: filter, Message: extractErrorMessage(jsErr)})
	return nil
}

// lostReasonFromJS maps the GPUDeviceLostInfo reason string to the internal
// reason. Only the spec's two values exist; anything else is Unknown, as the
// rust build maps a reason outside its C enum (C7 precedent).
func lostReasonFromJS(reason string) loststate.Reason {
	if reason == "destroyed" {
		return loststate.ReasonDestroyed
	}
	return loststate.ReasonUnknown
}

// jsStringProperty reads a string property, answering "" for an absent one.
func jsStringProperty(obj js.Value, name string) string {
	v := obj.Get(name)
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	return v.String()
}

// jsConstructorName returns a JS error's class name, such as
// "GPUValidationError"; "" when it has none.
func jsConstructorName(v js.Value) string {
	ctor := v.Get("constructor")
	if ctor.IsUndefined() || ctor.IsNull() {
		return ""
	}
	return jsStringProperty(ctor, "name")
}

// SetLostCallback replaces the device-lost registration. It never replays an
// event to a replaced registration, and a registration made on an already-lost
// device receives the stored info exactly once, at this call's return. nil
// clears the registration without delivering anything. Every callback is
// invoked after the state machine's lock is released, on the JS thread.
func (d *Device) SetLostCallback(fn func(loststate.Info)) {
	if d == nil || d.lost == nil {
		return
	}
	fire, ok := d.lost.SetLostCallback(fn)
	if ok {
		fire.Fn(fire.Info)
	}
	d.DispatchPending()
}

// SetUncapturedErrorCallback replaces the uncaptured-error registration. It
// never replays an already delivered error; nil clears the registration.
func (d *Device) SetUncapturedErrorCallback(fn func(*GPUErrorInfo)) {
	if d == nil || d.lost == nil {
		return
	}
	d.lost.SetUncapturedErrorCallback(fn)
}

// LostInfo reports the current lost state; ok is false on a live device.
func (d *Device) LostInfo() (loststate.Info, bool) {
	if d == nil || d.lost == nil {
		return loststate.Info{}, false
	}
	return d.lost.LostInfo()
}

// DispatchPending drains the deliveries the recording paths queued and invokes
// them in record order. It is called at the end of every handler, outside the
// state machine's lock, and is exported for the root declaration layer's entry
// points (C4: no callback is invoked while any lock is held).
func (d *Device) DispatchPending() {
	if d == nil || d.lost == nil {
		return
	}
	lostFires, errFires := d.lost.TakePending()
	for _, fire := range lostFires {
		fire.Fn(fire.Info)
	}
	for _, fire := range errFires {
		fire.Fn(fire.Err)
	}
}

// markLost latches a loss and dispatches it. loststate releases its lock before
// MarkLost returns, so the dispatch below — and the callback it invokes — runs
// with no lock held (C4).
func (d *Device) markLost(reason loststate.Reason, message string) {
	if d == nil || d.lost == nil {
		return
	}
	d.lost.MarkLost(reason, message)
	d.DispatchPending()
}

// recordUncaptured queues an uncaptured error and dispatches it, with the same
// record-then-dispatch separation as markLost.
func (d *Device) recordUncaptured(info *GPUErrorInfo) {
	if d == nil || d.lost == nil {
		return
	}
	d.lost.RecordUncaptured(info)
	d.DispatchPending()
}

// bindMethod returns methodName.bind(obj) so that Invoke() calls the method
// with the correct `this` context.
func bindMethod(obj js.Value, methodName string) js.Value {
	method := obj.Get(methodName)
	if method.IsUndefined() || method.IsNull() {
		// Method not available — return undefined. Callers will get a
		// clear JS error when they try to invoke it.
		return js.Undefined()
	}
	return method.Call("bind", obj)
}

// Queue returns the device's command queue.
func (d *Device) Queue() *Queue {
	return d.queue
}

// Features returns the device's GPUSupportedFeatures js.Value.
func (d *Device) Features() js.Value {
	return d.features
}

// Limits returns the device's GPUSupportedLimits js.Value.
func (d *Device) Limits() js.Value {
	return d.limits
}

// Ref returns the underlying GPUDevice js.Value.
func (d *Device) Ref() js.Value {
	return d.ref_
}

// CreateBuffer returns the pre-bound createBuffer function.
// Call with: d.CreateBuffer().Invoke(descriptorObj)
func (d *Device) CreateBuffer() js.Value { return d.fnCreateBuffer }

// CreateTexture returns the pre-bound createTexture function.
func (d *Device) CreateTexture() js.Value { return d.fnCreateTexture }

// CreateShaderModule returns the pre-bound createShaderModule function.
func (d *Device) CreateShaderModule() js.Value { return d.fnCreateShaderModule }

// CreateBindGroupLayout returns the pre-bound createBindGroupLayout function.
func (d *Device) CreateBindGroupLayout() js.Value { return d.fnCreateBindGroupLayout }

// CreateBindGroup returns the pre-bound createBindGroup function.
func (d *Device) CreateBindGroup() js.Value { return d.fnCreateBindGroup }

// CreatePipelineLayout returns the pre-bound createPipelineLayout function.
func (d *Device) CreatePipelineLayout() js.Value { return d.fnCreatePipelineLayout }

// CreateRenderPipeline returns the pre-bound createRenderPipeline function.
func (d *Device) CreateRenderPipeline() js.Value { return d.fnCreateRenderPipeline }

// CreateComputePipeline returns the pre-bound createComputePipeline function.
func (d *Device) CreateComputePipeline() js.Value { return d.fnCreateComputePipeline }

// CreateCommandEncoder returns the pre-bound createCommandEncoder function.
func (d *Device) CreateCommandEncoder() js.Value { return d.fnCreateCommandEncoder }

// CreateSampler returns the pre-bound createSampler function.
func (d *Device) CreateSampler() js.Value { return d.fnCreateSampler }

// CreateQuerySet returns the pre-bound createQuerySet function.
func (d *Device) CreateQuerySet() js.Value { return d.fnCreateQuerySet }

// CreateBufferFromDesc creates a GPUBuffer from a JS descriptor object.
func (d *Device) CreateBufferFromDesc(desc js.Value) *Buffer {
	jsBuffer := d.fnCreateBuffer.Invoke(desc)
	return NewBuffer(jsBuffer)
}

// CreateTextureFromDesc creates a GPUTexture from a JS descriptor object.
func (d *Device) CreateTextureFromDesc(desc js.Value) *Texture {
	jsTexture := d.fnCreateTexture.Invoke(desc)
	return NewTexture(jsTexture)
}

// CreateSamplerFromDesc creates a GPUSampler from a JS descriptor object.
func (d *Device) CreateSamplerFromDesc(desc js.Value) *Sampler {
	jsSampler := d.fnCreateSampler.Invoke(desc)
	return NewSampler(jsSampler)
}

// CreateShaderModuleFromDesc creates a GPUShaderModule from a JS descriptor object.
func (d *Device) CreateShaderModuleFromDesc(desc js.Value) *ShaderModule {
	jsModule := d.fnCreateShaderModule.Invoke(desc)
	return NewShaderModule(jsModule)
}

// CreateBindGroupLayoutFromDesc creates a GPUBindGroupLayout from a JS descriptor object.
func (d *Device) CreateBindGroupLayoutFromDesc(desc js.Value) *BindGroupLayout {
	jsLayout := d.fnCreateBindGroupLayout.Invoke(desc)
	return NewBindGroupLayout(jsLayout)
}

// CreateBindGroupFromDesc creates a GPUBindGroup from a JS descriptor object.
func (d *Device) CreateBindGroupFromDesc(desc js.Value) *BindGroup {
	jsGroup := d.fnCreateBindGroup.Invoke(desc)
	return NewBindGroup(jsGroup)
}

// CreatePipelineLayoutFromDesc creates a GPUPipelineLayout from a JS descriptor object.
func (d *Device) CreatePipelineLayoutFromDesc(desc js.Value) *PipelineLayout {
	jsLayout := d.fnCreatePipelineLayout.Invoke(desc)
	return NewPipelineLayout(jsLayout)
}

// CreateRenderPipelineFromDesc creates a GPURenderPipeline from a JS descriptor object.
func (d *Device) CreateRenderPipelineFromDesc(desc js.Value) *RenderPipeline {
	jsPipeline := d.fnCreateRenderPipeline.Invoke(desc)
	return NewRenderPipeline(jsPipeline)
}

// CreateComputePipelineFromDesc creates a GPUComputePipeline from a JS descriptor object.
func (d *Device) CreateComputePipelineFromDesc(desc js.Value) *ComputePipeline {
	jsPipeline := d.fnCreateComputePipeline.Invoke(desc)
	return NewComputePipeline(jsPipeline)
}

// Destroy calls GPUDevice.destroy() to release GPU resources.
// After this call the device is no longer usable.
func (d *Device) Destroy() {
	destroy := d.ref_.Get("destroy")
	if !destroy.IsUndefined() && !destroy.IsNull() {
		d.ref_.Call("destroy")
	}
	// GPUDevice.destroy() resolves the lost promise with reason "destroyed".
	// Recording it here as well makes the loss observable when Release returns,
	// before the promise's microtask runs (C10); first-wins keeps a driver loss
	// recorded earlier, and the later promise reaction becomes a no-op.
	d.markLost(loststate.ReasonDestroyed, "device destroyed")
	// Release the JS callback closures: the handlers stay bound on the JS
	// object otherwise, keeping the Go function values (and their userdata)
	// alive for every destroyed device (review P3-1). Release is safe on a zero
	// Func (the runtime's bubble release is nil-safe), so no guard is needed;
	// the event-handler property is unbound before its closure is released, and
	// the lost promise reaction and the event handler are inert after destroy.
	d.ref_.Set("onuncapturederror", js.Null())
	d.fnUncaptured.Release()
	d.fnUncaptured = js.Func{}
	d.fnLostResolve.Release()
	d.fnLostResolve = js.Func{}
}
