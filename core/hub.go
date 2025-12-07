package core

import (
	"sync"
)

// Hub manages all resource registries for WebGPU.
// It's the central storage for all GPU resources (adapters, devices, buffers, etc.).
//
// The Hub is organized by resource type, with each type having its own registry.
// This provides type-safe access to resources and automatic ID management.
//
// Thread-safe for concurrent use.
type Hub struct {
	mu sync.RWMutex

	adapters         *Registry[Adapter, adapterMarker]
	devices          *Registry[Device, deviceMarker]
	queues           *Registry[Queue, queueMarker]
	buffers          *Registry[Buffer, bufferMarker]
	textures         *Registry[Texture, textureMarker]
	textureViews     *Registry[TextureView, textureViewMarker]
	samplers         *Registry[Sampler, samplerMarker]
	bindGroupLayouts *Registry[BindGroupLayout, bindGroupLayoutMarker]
	pipelineLayouts  *Registry[PipelineLayout, pipelineLayoutMarker]
	bindGroups       *Registry[BindGroup, bindGroupMarker]
	shaderModules    *Registry[ShaderModule, shaderModuleMarker]
	renderPipelines  *Registry[RenderPipeline, renderPipelineMarker]
	computePipelines *Registry[ComputePipeline, computePipelineMarker]
	commandEncoders  *Registry[CommandEncoder, commandEncoderMarker]
	commandBuffers   *Registry[CommandBuffer, commandBufferMarker]
	querySets        *Registry[QuerySet, querySetMarker]
}

// NewHub creates a new hub with initialized registries for all resource types.
func NewHub() *Hub {
	return &Hub{
		adapters:         NewRegistry[Adapter, adapterMarker](),
		devices:          NewRegistry[Device, deviceMarker](),
		queues:           NewRegistry[Queue, queueMarker](),
		buffers:          NewRegistry[Buffer, bufferMarker](),
		textures:         NewRegistry[Texture, textureMarker](),
		textureViews:     NewRegistry[TextureView, textureViewMarker](),
		samplers:         NewRegistry[Sampler, samplerMarker](),
		bindGroupLayouts: NewRegistry[BindGroupLayout, bindGroupLayoutMarker](),
		pipelineLayouts:  NewRegistry[PipelineLayout, pipelineLayoutMarker](),
		bindGroups:       NewRegistry[BindGroup, bindGroupMarker](),
		shaderModules:    NewRegistry[ShaderModule, shaderModuleMarker](),
		renderPipelines:  NewRegistry[RenderPipeline, renderPipelineMarker](),
		computePipelines: NewRegistry[ComputePipeline, computePipelineMarker](),
		commandEncoders:  NewRegistry[CommandEncoder, commandEncoderMarker](),
		commandBuffers:   NewRegistry[CommandBuffer, commandBufferMarker](),
		querySets:        NewRegistry[QuerySet, querySetMarker](),
	}
}

// Adapter methods

// RegisterAdapter allocates a new ID and stores the adapter.
func (h *Hub) RegisterAdapter(adapter *Adapter) AdapterID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.adapters.Register(*adapter)
}

// GetAdapter retrieves an adapter by ID.
func (h *Hub) GetAdapter(id AdapterID) (Adapter, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.adapters.Get(id)
}

// UnregisterAdapter removes an adapter by ID.
func (h *Hub) UnregisterAdapter(id AdapterID) (Adapter, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.adapters.Unregister(id)
}

// Device methods

// RegisterDevice allocates a new ID and stores the device.
func (h *Hub) RegisterDevice(device Device) DeviceID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.devices.Register(device)
}

// GetDevice retrieves a device by ID.
func (h *Hub) GetDevice(id DeviceID) (Device, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.devices.Get(id)
}

// UnregisterDevice removes a device by ID.
func (h *Hub) UnregisterDevice(id DeviceID) (Device, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.devices.Unregister(id)
}

// Queue methods

// RegisterQueue allocates a new ID and stores the queue.
func (h *Hub) RegisterQueue(queue Queue) QueueID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.queues.Register(queue)
}

// GetQueue retrieves a queue by ID.
func (h *Hub) GetQueue(id QueueID) (Queue, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.queues.Get(id)
}

// UnregisterQueue removes a queue by ID.
func (h *Hub) UnregisterQueue(id QueueID) (Queue, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.queues.Unregister(id)
}

// UpdateQueue updates a queue by ID.
func (h *Hub) UpdateQueue(id QueueID, queue Queue) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.queues.GetMut(id, func(q *Queue) {
		*q = queue
	})
}

// Buffer methods

// RegisterBuffer allocates a new ID and stores the buffer.
func (h *Hub) RegisterBuffer(buffer Buffer) BufferID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buffers.Register(buffer)
}

// GetBuffer retrieves a buffer by ID.
func (h *Hub) GetBuffer(id BufferID) (Buffer, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.buffers.Get(id)
}

// UnregisterBuffer removes a buffer by ID.
func (h *Hub) UnregisterBuffer(id BufferID) (Buffer, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.buffers.Unregister(id)
}

// Texture methods

// RegisterTexture allocates a new ID and stores the texture.
func (h *Hub) RegisterTexture(texture Texture) TextureID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.textures.Register(texture)
}

// GetTexture retrieves a texture by ID.
func (h *Hub) GetTexture(id TextureID) (Texture, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.textures.Get(id)
}

// UnregisterTexture removes a texture by ID.
func (h *Hub) UnregisterTexture(id TextureID) (Texture, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.textures.Unregister(id)
}

// TextureView methods

// RegisterTextureView allocates a new ID and stores the texture view.
func (h *Hub) RegisterTextureView(view TextureView) TextureViewID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.textureViews.Register(view)
}

// GetTextureView retrieves a texture view by ID.
func (h *Hub) GetTextureView(id TextureViewID) (TextureView, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.textureViews.Get(id)
}

// UnregisterTextureView removes a texture view by ID.
func (h *Hub) UnregisterTextureView(id TextureViewID) (TextureView, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.textureViews.Unregister(id)
}

// Sampler methods

// RegisterSampler allocates a new ID and stores the sampler.
func (h *Hub) RegisterSampler(sampler Sampler) SamplerID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.samplers.Register(sampler)
}

// GetSampler retrieves a sampler by ID.
func (h *Hub) GetSampler(id SamplerID) (Sampler, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.samplers.Get(id)
}

// UnregisterSampler removes a sampler by ID.
func (h *Hub) UnregisterSampler(id SamplerID) (Sampler, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.samplers.Unregister(id)
}

// BindGroupLayout methods

// RegisterBindGroupLayout allocates a new ID and stores the bind group layout.
func (h *Hub) RegisterBindGroupLayout(layout BindGroupLayout) BindGroupLayoutID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bindGroupLayouts.Register(layout)
}

// GetBindGroupLayout retrieves a bind group layout by ID.
func (h *Hub) GetBindGroupLayout(id BindGroupLayoutID) (BindGroupLayout, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bindGroupLayouts.Get(id)
}

// UnregisterBindGroupLayout removes a bind group layout by ID.
func (h *Hub) UnregisterBindGroupLayout(id BindGroupLayoutID) (BindGroupLayout, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bindGroupLayouts.Unregister(id)
}

// PipelineLayout methods

// RegisterPipelineLayout allocates a new ID and stores the pipeline layout.
func (h *Hub) RegisterPipelineLayout(layout PipelineLayout) PipelineLayoutID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pipelineLayouts.Register(layout)
}

// GetPipelineLayout retrieves a pipeline layout by ID.
func (h *Hub) GetPipelineLayout(id PipelineLayoutID) (PipelineLayout, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.pipelineLayouts.Get(id)
}

// UnregisterPipelineLayout removes a pipeline layout by ID.
func (h *Hub) UnregisterPipelineLayout(id PipelineLayoutID) (PipelineLayout, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.pipelineLayouts.Unregister(id)
}

// BindGroup methods

// RegisterBindGroup allocates a new ID and stores the bind group.
func (h *Hub) RegisterBindGroup(group BindGroup) BindGroupID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bindGroups.Register(group)
}

// GetBindGroup retrieves a bind group by ID.
func (h *Hub) GetBindGroup(id BindGroupID) (BindGroup, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bindGroups.Get(id)
}

// UnregisterBindGroup removes a bind group by ID.
func (h *Hub) UnregisterBindGroup(id BindGroupID) (BindGroup, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.bindGroups.Unregister(id)
}

// ShaderModule methods

// RegisterShaderModule allocates a new ID and stores the shader module.
func (h *Hub) RegisterShaderModule(module ShaderModule) ShaderModuleID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.shaderModules.Register(module)
}

// GetShaderModule retrieves a shader module by ID.
func (h *Hub) GetShaderModule(id ShaderModuleID) (ShaderModule, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.shaderModules.Get(id)
}

// UnregisterShaderModule removes a shader module by ID.
func (h *Hub) UnregisterShaderModule(id ShaderModuleID) (ShaderModule, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.shaderModules.Unregister(id)
}

// RenderPipeline methods

// RegisterRenderPipeline allocates a new ID and stores the render pipeline.
func (h *Hub) RegisterRenderPipeline(pipeline RenderPipeline) RenderPipelineID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.renderPipelines.Register(pipeline)
}

// GetRenderPipeline retrieves a render pipeline by ID.
func (h *Hub) GetRenderPipeline(id RenderPipelineID) (RenderPipeline, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.renderPipelines.Get(id)
}

// UnregisterRenderPipeline removes a render pipeline by ID.
func (h *Hub) UnregisterRenderPipeline(id RenderPipelineID) (RenderPipeline, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.renderPipelines.Unregister(id)
}

// ComputePipeline methods

// RegisterComputePipeline allocates a new ID and stores the compute pipeline.
func (h *Hub) RegisterComputePipeline(pipeline ComputePipeline) ComputePipelineID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.computePipelines.Register(pipeline)
}

// GetComputePipeline retrieves a compute pipeline by ID.
func (h *Hub) GetComputePipeline(id ComputePipelineID) (ComputePipeline, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.computePipelines.Get(id)
}

// UnregisterComputePipeline removes a compute pipeline by ID.
func (h *Hub) UnregisterComputePipeline(id ComputePipelineID) (ComputePipeline, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.computePipelines.Unregister(id)
}

// CommandEncoder methods

// RegisterCommandEncoder allocates a new ID and stores the command encoder.
func (h *Hub) RegisterCommandEncoder(encoder CommandEncoder) CommandEncoderID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.commandEncoders.Register(encoder)
}

// GetCommandEncoder retrieves a command encoder by ID.
func (h *Hub) GetCommandEncoder(id CommandEncoderID) (CommandEncoder, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.commandEncoders.Get(id)
}

// UnregisterCommandEncoder removes a command encoder by ID.
func (h *Hub) UnregisterCommandEncoder(id CommandEncoderID) (CommandEncoder, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.commandEncoders.Unregister(id)
}

// CommandBuffer methods

// RegisterCommandBuffer allocates a new ID and stores the command buffer.
func (h *Hub) RegisterCommandBuffer(buffer CommandBuffer) CommandBufferID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.commandBuffers.Register(buffer)
}

// GetCommandBuffer retrieves a command buffer by ID.
func (h *Hub) GetCommandBuffer(id CommandBufferID) (CommandBuffer, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.commandBuffers.Get(id)
}

// UnregisterCommandBuffer removes a command buffer by ID.
func (h *Hub) UnregisterCommandBuffer(id CommandBufferID) (CommandBuffer, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.commandBuffers.Unregister(id)
}

// QuerySet methods

// RegisterQuerySet allocates a new ID and stores the query set.
func (h *Hub) RegisterQuerySet(querySet QuerySet) QuerySetID {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.querySets.Register(querySet)
}

// GetQuerySet retrieves a query set by ID.
func (h *Hub) GetQuerySet(id QuerySetID) (QuerySet, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.querySets.Get(id)
}

// UnregisterQuerySet removes a query set by ID.
func (h *Hub) UnregisterQuerySet(id QuerySetID) (QuerySet, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.querySets.Unregister(id)
}

// ResourceCounts returns the count of each resource type in the hub.
// Useful for debugging and diagnostics.
func (h *Hub) ResourceCounts() map[string]uint64 {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return map[string]uint64{
		"adapters":         h.adapters.Count(),
		"devices":          h.devices.Count(),
		"queues":           h.queues.Count(),
		"buffers":          h.buffers.Count(),
		"textures":         h.textures.Count(),
		"textureViews":     h.textureViews.Count(),
		"samplers":         h.samplers.Count(),
		"bindGroupLayouts": h.bindGroupLayouts.Count(),
		"pipelineLayouts":  h.pipelineLayouts.Count(),
		"bindGroups":       h.bindGroups.Count(),
		"shaderModules":    h.shaderModules.Count(),
		"renderPipelines":  h.renderPipelines.Count(),
		"computePipelines": h.computePipelines.Count(),
		"commandEncoders":  h.commandEncoders.Count(),
		"commandBuffers":   h.commandBuffers.Count(),
		"querySets":        h.querySets.Count(),
	}
}

// Clear removes all resources from the hub.
// Note: This does not release IDs properly - use only for cleanup/testing.
func (h *Hub) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.adapters.Clear()
	h.devices.Clear()
	h.queues.Clear()
	h.buffers.Clear()
	h.textures.Clear()
	h.textureViews.Clear()
	h.samplers.Clear()
	h.bindGroupLayouts.Clear()
	h.pipelineLayouts.Clear()
	h.bindGroups.Clear()
	h.shaderModules.Clear()
	h.renderPipelines.Clear()
	h.computePipelines.Clear()
	h.commandEncoders.Clear()
	h.commandBuffers.Clear()
	h.querySets.Clear()
}
