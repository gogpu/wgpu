package core

import (
	"errors"
	"sync"
	"testing"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	// Verify all registries are initialized
	counts := hub.ResourceCounts()
	expectedTypes := []string{
		"adapters", "devices", "queues", "buffers",
		"textures", "textureViews", "samplers",
		"bindGroupLayouts", "pipelineLayouts", "bindGroups",
		"shaderModules", "renderPipelines", "computePipelines",
		"commandEncoders", "commandBuffers", "querySets",
	}

	for _, resourceType := range expectedTypes {
		count, ok := counts[resourceType]
		if !ok {
			t.Errorf("ResourceCounts missing %s", resourceType)
		}
		if count != 0 {
			t.Errorf("Initial count for %s = %d, want 0", resourceType, count)
		}
	}
}

func TestHubAdapter(t *testing.T) {
	hub := NewHub()
	adapter := &Adapter{}

	// Register
	id := hub.RegisterAdapter(adapter)
	if id.IsZero() {
		t.Fatal("RegisterAdapter returned zero ID")
	}

	// Get
	got, err := hub.GetAdapter(id)
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}
	if got != *adapter {
		t.Error("GetAdapter returned different adapter")
	}

	// Unregister
	removed, err := hub.UnregisterAdapter(id)
	if err != nil {
		t.Fatalf("UnregisterAdapter failed: %v", err)
	}
	if removed != *adapter {
		t.Error("UnregisterAdapter returned different adapter")
	}

	// Get after unregister should fail
	_, err = hub.GetAdapter(id)
	if err == nil {
		t.Error("GetAdapter after unregister should fail")
	}
}

func TestHubDevice(t *testing.T) {
	hub := NewHub()
	device := Device{}

	id := hub.RegisterDevice(device)
	got, err := hub.GetDevice(id)
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	if got != device {
		t.Error("GetDevice returned different device")
	}

	_, err = hub.UnregisterDevice(id)
	if err != nil {
		t.Fatalf("UnregisterDevice failed: %v", err)
	}
}

func TestHubQueue(t *testing.T) {
	hub := NewHub()
	queue := Queue{}

	id := hub.RegisterQueue(queue)
	got, err := hub.GetQueue(id)
	if err != nil {
		t.Fatalf("GetQueue failed: %v", err)
	}
	if got != queue {
		t.Error("GetQueue returned different queue")
	}

	_, err = hub.UnregisterQueue(id)
	if err != nil {
		t.Fatalf("UnregisterQueue failed: %v", err)
	}
}

func TestHubBuffer(t *testing.T) {
	hub := NewHub()
	buffer := Buffer{
		label: "TestBuffer",
		size:  1024,
	}

	id := hub.RegisterBuffer(buffer)
	got, err := hub.GetBuffer(id)
	if err != nil {
		t.Fatalf("GetBuffer failed: %v", err)
	}
	// Can't compare structs with sync.Map, compare fields instead
	if got.label != buffer.label || got.size != buffer.size {
		t.Error("GetBuffer returned different buffer")
	}

	_, err = hub.UnregisterBuffer(id)
	if err != nil {
		t.Fatalf("UnregisterBuffer failed: %v", err)
	}
}

func TestHubTexture(t *testing.T) {
	hub := NewHub()
	texture := Texture{}

	id := hub.RegisterTexture(texture)
	got, err := hub.GetTexture(id)
	if err != nil {
		t.Fatalf("GetTexture failed: %v", err)
	}
	if got != texture {
		t.Error("GetTexture returned different texture")
	}

	_, err = hub.UnregisterTexture(id)
	if err != nil {
		t.Fatalf("UnregisterTexture failed: %v", err)
	}
}

func TestHubTextureView(t *testing.T) {
	hub := NewHub()
	view := TextureView{}

	id := hub.RegisterTextureView(view)
	got, err := hub.GetTextureView(id)
	if err != nil {
		t.Fatalf("GetTextureView failed: %v", err)
	}
	if got != view {
		t.Error("GetTextureView returned different view")
	}

	_, err = hub.UnregisterTextureView(id)
	if err != nil {
		t.Fatalf("UnregisterTextureView failed: %v", err)
	}
}

func TestHubSampler(t *testing.T) {
	hub := NewHub()
	sampler := Sampler{}

	id := hub.RegisterSampler(sampler)
	got, err := hub.GetSampler(id)
	if err != nil {
		t.Fatalf("GetSampler failed: %v", err)
	}
	if got != sampler {
		t.Error("GetSampler returned different sampler")
	}

	_, err = hub.UnregisterSampler(id)
	if err != nil {
		t.Fatalf("UnregisterSampler failed: %v", err)
	}
}

func TestHubBindGroupLayout(t *testing.T) {
	hub := NewHub()
	layout := BindGroupLayout{}

	id := hub.RegisterBindGroupLayout(layout)
	got, err := hub.GetBindGroupLayout(id)
	if err != nil {
		t.Fatalf("GetBindGroupLayout failed: %v", err)
	}
	if got != layout {
		t.Error("GetBindGroupLayout returned different layout")
	}

	_, err = hub.UnregisterBindGroupLayout(id)
	if err != nil {
		t.Fatalf("UnregisterBindGroupLayout failed: %v", err)
	}
}

func TestHubPipelineLayout(t *testing.T) {
	hub := NewHub()
	layout := PipelineLayout{}

	id := hub.RegisterPipelineLayout(layout)
	got, err := hub.GetPipelineLayout(id)
	if err != nil {
		t.Fatalf("GetPipelineLayout failed: %v", err)
	}
	if got != layout {
		t.Error("GetPipelineLayout returned different layout")
	}

	_, err = hub.UnregisterPipelineLayout(id)
	if err != nil {
		t.Fatalf("UnregisterPipelineLayout failed: %v", err)
	}
}

func TestHubBindGroup(t *testing.T) {
	hub := NewHub()
	group := BindGroup{}

	id := hub.RegisterBindGroup(group)
	got, err := hub.GetBindGroup(id)
	if err != nil {
		t.Fatalf("GetBindGroup failed: %v", err)
	}
	if got != group {
		t.Error("GetBindGroup returned different group")
	}

	_, err = hub.UnregisterBindGroup(id)
	if err != nil {
		t.Fatalf("UnregisterBindGroup failed: %v", err)
	}
}

func TestHubShaderModule(t *testing.T) {
	hub := NewHub()
	module := ShaderModule{}

	id := hub.RegisterShaderModule(module)
	got, err := hub.GetShaderModule(id)
	if err != nil {
		t.Fatalf("GetShaderModule failed: %v", err)
	}
	if got != module {
		t.Error("GetShaderModule returned different module")
	}

	_, err = hub.UnregisterShaderModule(id)
	if err != nil {
		t.Fatalf("UnregisterShaderModule failed: %v", err)
	}
}

func TestHubRenderPipeline(t *testing.T) {
	hub := NewHub()
	pipeline := RenderPipeline{}

	id := hub.RegisterRenderPipeline(pipeline)
	got, err := hub.GetRenderPipeline(id)
	if err != nil {
		t.Fatalf("GetRenderPipeline failed: %v", err)
	}
	if got != pipeline {
		t.Error("GetRenderPipeline returned different pipeline")
	}

	_, err = hub.UnregisterRenderPipeline(id)
	if err != nil {
		t.Fatalf("UnregisterRenderPipeline failed: %v", err)
	}
}

func TestHubComputePipeline(t *testing.T) {
	hub := NewHub()
	pipeline := ComputePipeline{}

	id := hub.RegisterComputePipeline(pipeline)
	got, err := hub.GetComputePipeline(id)
	if err != nil {
		t.Fatalf("GetComputePipeline failed: %v", err)
	}
	if got != pipeline {
		t.Error("GetComputePipeline returned different pipeline")
	}

	_, err = hub.UnregisterComputePipeline(id)
	if err != nil {
		t.Fatalf("UnregisterComputePipeline failed: %v", err)
	}
}

func TestHubCommandEncoder(t *testing.T) {
	hub := NewHub()
	encoder := CommandEncoder{}

	id := hub.RegisterCommandEncoder(encoder)
	got, err := hub.GetCommandEncoder(id)
	if err != nil {
		t.Fatalf("GetCommandEncoder failed: %v", err)
	}
	if got != encoder {
		t.Error("GetCommandEncoder returned different encoder")
	}

	_, err = hub.UnregisterCommandEncoder(id)
	if err != nil {
		t.Fatalf("UnregisterCommandEncoder failed: %v", err)
	}
}

func TestHubCommandBuffer(t *testing.T) {
	hub := NewHub()
	buffer := CommandBuffer{}

	id := hub.RegisterCommandBuffer(buffer)
	got, err := hub.GetCommandBuffer(id)
	if err != nil {
		t.Fatalf("GetCommandBuffer failed: %v", err)
	}
	if got != buffer {
		t.Error("GetCommandBuffer returned different buffer")
	}

	_, err = hub.UnregisterCommandBuffer(id)
	if err != nil {
		t.Fatalf("UnregisterCommandBuffer failed: %v", err)
	}
}

func TestHubQuerySet(t *testing.T) {
	hub := NewHub()
	querySet := QuerySet{}

	id := hub.RegisterQuerySet(querySet)
	got, err := hub.GetQuerySet(id)
	if err != nil {
		t.Fatalf("GetQuerySet failed: %v", err)
	}
	if got != querySet {
		t.Error("GetQuerySet returned different query set")
	}

	_, err = hub.UnregisterQuerySet(id)
	if err != nil {
		t.Fatalf("UnregisterQuerySet failed: %v", err)
	}
}

func TestHubResourceCounts(t *testing.T) {
	hub := NewHub()

	// Register one of each type
	hub.RegisterAdapter(&Adapter{})
	hub.RegisterDevice(Device{})
	hub.RegisterQueue(Queue{})
	hub.RegisterBuffer(Buffer{})
	hub.RegisterTexture(Texture{})
	hub.RegisterTextureView(TextureView{})
	hub.RegisterSampler(Sampler{})
	hub.RegisterBindGroupLayout(BindGroupLayout{})
	hub.RegisterPipelineLayout(PipelineLayout{})
	hub.RegisterBindGroup(BindGroup{})
	hub.RegisterShaderModule(ShaderModule{})
	hub.RegisterRenderPipeline(RenderPipeline{})
	hub.RegisterComputePipeline(ComputePipeline{})
	hub.RegisterCommandEncoder(CommandEncoder{})
	hub.RegisterCommandBuffer(CommandBuffer{})
	hub.RegisterQuerySet(QuerySet{})

	counts := hub.ResourceCounts()
	expectedCount := uint64(1)
	for resourceType, count := range counts {
		if count != expectedCount {
			t.Errorf("%s count = %d, want %d", resourceType, count, expectedCount)
		}
	}
}

func TestHubClear(t *testing.T) {
	hub := NewHub()

	// Register resources
	adapterID := hub.RegisterAdapter(&Adapter{})
	deviceID := hub.RegisterDevice(Device{})
	bufferID := hub.RegisterBuffer(Buffer{})

	// Verify they exist
	_, err := hub.GetAdapter(adapterID)
	if err != nil {
		t.Fatalf("GetAdapter failed: %v", err)
	}
	_, err = hub.GetDevice(deviceID)
	if err != nil {
		t.Fatalf("GetDevice failed: %v", err)
	}
	_, err = hub.GetBuffer(bufferID)
	if err != nil {
		t.Fatalf("GetBuffer failed: %v", err)
	}

	// Clear removes storage but doesn't reset counts
	// (as noted in the Clear() comment: "does not release IDs properly")
	hub.Clear()

	// Verify resources are no longer accessible (storage cleared)
	_, err = hub.GetAdapter(adapterID)
	if err == nil {
		t.Error("GetAdapter should fail after Clear")
	}
	_, err = hub.GetDevice(deviceID)
	if err == nil {
		t.Error("GetDevice should fail after Clear")
	}
	_, err = hub.GetBuffer(bufferID)
	if err == nil {
		t.Error("GetBuffer should fail after Clear")
	}
}

func TestHubConcurrentAccess(t *testing.T) {
	hub := NewHub()
	const goroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				// Register
				id := hub.RegisterBuffer(Buffer{})

				// Get
				_, err := hub.GetBuffer(id)
				if err != nil {
					t.Errorf("GetBuffer failed: %v", err)
				}

				// Unregister
				_, err = hub.UnregisterBuffer(id)
				if err != nil {
					t.Errorf("UnregisterBuffer failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// All buffers should be unregistered
	counts := hub.ResourceCounts()
	if counts["buffers"] != 0 {
		t.Errorf("After concurrent test, buffer count = %d, want 0", counts["buffers"])
	}
}

func TestHubMultipleResources(t *testing.T) {
	hub := NewHub()

	// Register multiple resources of different types
	adapterID1 := hub.RegisterAdapter(&Adapter{})
	adapterID2 := hub.RegisterAdapter(&Adapter{})
	deviceID := hub.RegisterDevice(Device{})
	bufferID := hub.RegisterBuffer(Buffer{})

	// Verify all are accessible
	if _, err := hub.GetAdapter(adapterID1); err != nil {
		t.Errorf("GetAdapter(1) failed: %v", err)
	}
	if _, err := hub.GetAdapter(adapterID2); err != nil {
		t.Errorf("GetAdapter(2) failed: %v", err)
	}
	if _, err := hub.GetDevice(deviceID); err != nil {
		t.Errorf("GetDevice failed: %v", err)
	}
	if _, err := hub.GetBuffer(bufferID); err != nil {
		t.Errorf("GetBuffer failed: %v", err)
	}

	// Verify counts
	counts := hub.ResourceCounts()
	if counts["adapters"] != 2 {
		t.Errorf("adapter count = %d, want 2", counts["adapters"])
	}
	if counts["devices"] != 1 {
		t.Errorf("device count = %d, want 1", counts["devices"])
	}
	if counts["buffers"] != 1 {
		t.Errorf("buffer count = %d, want 1", counts["buffers"])
	}
}

func TestHubInvalidID(t *testing.T) {
	hub := NewHub()

	// Try to get with zero ID
	zeroID := BufferID{}
	_, err := hub.GetBuffer(zeroID)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("GetBuffer with zero ID: got error %v, want %v", err, ErrInvalidID)
	}

	// Try to unregister with zero ID
	_, err = hub.UnregisterBuffer(zeroID)
	if !errors.Is(err, ErrInvalidID) {
		t.Errorf("UnregisterBuffer with zero ID: got error %v, want %v", err, ErrInvalidID)
	}
}

func TestHubEpochMismatch(t *testing.T) {
	hub := NewHub()

	// Register and unregister to increment epoch
	id1 := hub.RegisterBuffer(Buffer{})
	_, err := hub.UnregisterBuffer(id1)
	if err != nil {
		t.Fatalf("UnregisterBuffer failed: %v", err)
	}

	// Try to get with old ID (epoch mismatch)
	_, err = hub.GetBuffer(id1)
	if !errors.Is(err, ErrEpochMismatch) {
		t.Errorf("GetBuffer with old ID: got error %v, want %v", err, ErrEpochMismatch)
	}
}
