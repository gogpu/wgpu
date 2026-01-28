package core

import (
	"sync"
	"testing"

	"github.com/gogpu/gputypes"
)

func TestGetQueue(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() QueueID
		wantErr bool
	}{
		{
			name: "get valid queue",
			setup: func() QueueID {
				ResetGlobal()
				adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				device, _ := GetDevice(deviceID)
				return device.Queue
			},
			wantErr: false,
		},
		{
			name: "get invalid queue",
			setup: func() QueueID {
				ResetGlobal()
				return QueueID{} // Invalid ID
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queueID := tt.setup()
			queue, err := GetQueue(queueID)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetQueue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && queue == nil {
				t.Errorf("GetQueue() returned nil queue")
			}
		})
	}
}

func TestQueueSubmit(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	queueID, err := GetDeviceQueue(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceQueue() error = %v", err)
	}

	// Create a command buffer (placeholder)
	hub := GetGlobal().Hub()
	cmdBuf := CommandBuffer{}
	cmdBufID := hub.RegisterCommandBuffer(cmdBuf)

	tests := []struct {
		name           string
		queueID        QueueID
		commandBuffers []CommandBufferID
		wantErr        bool
	}{
		{
			name:           "submit empty command buffers",
			queueID:        queueID,
			commandBuffers: []CommandBufferID{},
			wantErr:        false,
		},
		{
			name:           "submit valid command buffer",
			queueID:        queueID,
			commandBuffers: []CommandBufferID{cmdBufID},
			wantErr:        false,
		},
		{
			name:           "submit with invalid queue",
			queueID:        QueueID{},
			commandBuffers: []CommandBufferID{cmdBufID},
			wantErr:        true,
		},
		{
			name:           "submit invalid command buffer",
			queueID:        queueID,
			commandBuffers: []CommandBufferID{CommandBufferID{}},
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := QueueSubmit(tt.queueID, tt.commandBuffers)

			if (err != nil) != tt.wantErr {
				t.Errorf("QueueSubmit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueueWriteBuffer(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	queueID, err := GetDeviceQueue(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceQueue() error = %v", err)
	}

	// Create a buffer (placeholder)
	bufferDesc := &gputypes.BufferDescriptor{
		Label: "Test Buffer",
		Size:  256,
		Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
	}
	bufferID, err := DeviceCreateBuffer(deviceID, bufferDesc)
	if err != nil {
		t.Fatalf("DeviceCreateBuffer() error = %v", err)
	}

	tests := []struct {
		name     string
		queueID  QueueID
		bufferID BufferID
		offset   uint64
		data     []byte
		wantErr  bool
	}{
		{
			name:     "write buffer with valid data",
			queueID:  queueID,
			bufferID: bufferID,
			offset:   0,
			data:     []byte{1, 2, 3, 4},
			wantErr:  false,
		},
		{
			name:     "write buffer with offset",
			queueID:  queueID,
			bufferID: bufferID,
			offset:   64,
			data:     []byte{5, 6, 7, 8},
			wantErr:  false,
		},
		{
			name:     "write with invalid queue",
			queueID:  QueueID{},
			bufferID: bufferID,
			offset:   0,
			data:     []byte{1, 2, 3, 4},
			wantErr:  true,
		},
		{
			name:     "write with invalid buffer",
			queueID:  queueID,
			bufferID: BufferID{},
			offset:   0,
			data:     []byte{1, 2, 3, 4},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := QueueWriteBuffer(tt.queueID, tt.bufferID, tt.offset, tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("QueueWriteBuffer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueueWriteTexture(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	queueID, err := GetDeviceQueue(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceQueue() error = %v", err)
	}

	// Create a texture (placeholder)
	textureDesc := &gputypes.TextureDescriptor{
		Label: "Test Texture",
		Size: gputypes.Extent3D{
			Width:              256,
			Height:             256,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	}
	textureID, err := DeviceCreateTexture(deviceID, textureDesc)
	if err != nil {
		t.Fatalf("DeviceCreateTexture() error = %v", err)
	}

	validDst := &gputypes.ImageCopyTexture{
		Texture:  uintptr(textureID.Raw()),
		MipLevel: 0,
		Origin:   gputypes.Origin3D{X: 0, Y: 0, Z: 0},
		Aspect:   gputypes.TextureAspectAll,
	}

	validLayout := &gputypes.TextureDataLayout{
		Offset:       0,
		BytesPerRow:  256 * 4,
		RowsPerImage: 256,
	}

	validSize := &gputypes.Extent3D{
		Width:              256,
		Height:             256,
		DepthOrArrayLayers: 1,
	}

	data := make([]byte, 256*256*4)

	tests := []struct {
		name    string
		queueID QueueID
		dst     *gputypes.ImageCopyTexture
		data    []byte
		layout  *gputypes.TextureDataLayout
		size    *gputypes.Extent3D
		wantErr bool
	}{
		{
			name:    "write texture with valid data",
			queueID: queueID,
			dst:     validDst,
			data:    data,
			layout:  validLayout,
			size:    validSize,
			wantErr: false,
		},
		{
			name:    "write with invalid queue",
			queueID: QueueID{},
			dst:     validDst,
			data:    data,
			layout:  validLayout,
			size:    validSize,
			wantErr: true,
		},
		{
			name:    "write with nil destination",
			queueID: queueID,
			dst:     nil,
			data:    data,
			layout:  validLayout,
			size:    validSize,
			wantErr: true,
		},
		{
			name:    "write with nil layout",
			queueID: queueID,
			dst:     validDst,
			data:    data,
			layout:  nil,
			size:    validSize,
			wantErr: true,
		},
		{
			name:    "write with nil size",
			queueID: queueID,
			dst:     validDst,
			data:    data,
			layout:  validLayout,
			size:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := QueueWriteTexture(tt.queueID, tt.dst, tt.data, tt.layout, tt.size)

			if (err != nil) != tt.wantErr {
				t.Errorf("QueueWriteTexture() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueueOnSubmittedWorkDone(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() QueueID
		wantErr bool
	}{
		{
			name: "wait for work on valid queue",
			setup: func() QueueID {
				ResetGlobal()
				adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
				deviceID, _ := CreateDevice(adapterID, nil)
				device, _ := GetDevice(deviceID)
				return device.Queue
			},
			wantErr: false,
		},
		{
			name: "wait for work on invalid queue",
			setup: func() QueueID {
				ResetGlobal()
				return QueueID{} // Invalid ID
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queueID := tt.setup()
			err := QueueOnSubmittedWorkDone(queueID)

			if (err != nil) != tt.wantErr {
				t.Errorf("QueueOnSubmittedWorkDone() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueueConcurrentOperations(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	queueID, err := GetDeviceQueue(deviceID)
	if err != nil {
		t.Fatalf("GetDeviceQueue() error = %v", err)
	}

	// Create buffers
	const numBuffers = 10
	bufferIDs := make([]BufferID, numBuffers)
	for i := 0; i < numBuffers; i++ {
		desc := &gputypes.BufferDescriptor{
			Label: "Concurrent Buffer",
			Size:  256,
			Usage: gputypes.BufferUsageVertex | gputypes.BufferUsageCopyDst,
		}
		bufferIDs[i], err = DeviceCreateBuffer(deviceID, desc)
		if err != nil {
			t.Fatalf("DeviceCreateBuffer() error = %v", err)
		}
	}

	// Write to buffers concurrently
	var wg sync.WaitGroup
	errors := make([]error, numBuffers)

	for i := 0; i < numBuffers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte{byte(idx), byte(idx + 1), byte(idx + 2), byte(idx + 3)}
			errors[idx] = QueueWriteBuffer(queueID, bufferIDs[idx], 0, data)
		}(i)
	}

	wg.Wait()

	// Verify all writes succeeded
	for i, err := range errors {
		if err != nil {
			t.Errorf("Buffer %d write failed: %v", i, err)
		}
	}
}

func TestQueueLifecycle(t *testing.T) {
	ResetGlobal()

	adapterID := createTestAdapter(t, gputypes.Features(0), gputypes.DefaultLimits())
	deviceID, err := CreateDevice(adapterID, nil)
	if err != nil {
		t.Fatalf("CreateDevice() error = %v", err)
	}

	device, err := GetDevice(deviceID)
	if err != nil {
		t.Fatalf("GetDevice() error = %v", err)
	}

	queueID := device.Queue

	// Verify queue exists
	queue, err := GetQueue(queueID)
	if err != nil {
		t.Fatalf("GetQueue() error = %v", err)
	}

	if queue.Device != deviceID {
		t.Errorf("Queue.Device = %v, want %v", queue.Device, deviceID)
	}

	// Drop device (should also drop queue)
	err = DeviceDrop(deviceID)
	if err != nil {
		t.Fatalf("DeviceDrop() error = %v", err)
	}

	// Verify queue no longer exists
	_, err = GetQueue(queueID)
	if err == nil {
		t.Errorf("GetQueue() should fail after device drop")
	}
}
