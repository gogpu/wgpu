// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package d3d12

// GUID represents a Windows GUID (Globally Unique Identifier).
// Layout must match Windows GUID structure exactly.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// D3D12 Interface GUIDs
// Reference: https://learn.microsoft.com/en-us/windows/win32/api/d3d12/

// IID_ID3D12Object is the interface ID for ID3D12Object.
// {C4FEC28F-7966-4E95-9F94-F431CB56C3B8}
var IID_ID3D12Object = GUID{
	Data1: 0xC4FEC28F,
	Data2: 0x7966,
	Data3: 0x4E95,
	Data4: [8]byte{0x9F, 0x94, 0xF4, 0x31, 0xCB, 0x56, 0xC3, 0xB8},
}

// IID_ID3D12DeviceChild is the interface ID for ID3D12DeviceChild.
// {905DB94B-A00C-4140-9DF5-2B64CA9EA357}
var IID_ID3D12DeviceChild = GUID{
	Data1: 0x905DB94B,
	Data2: 0xA00C,
	Data3: 0x4140,
	Data4: [8]byte{0x9D, 0xF5, 0x2B, 0x64, 0xCA, 0x9E, 0xA3, 0x57},
}

// IID_ID3D12Pageable is the interface ID for ID3D12Pageable.
// {63EE58FB-1268-4835-86DA-F008CE62F0D6}
var IID_ID3D12Pageable = GUID{
	Data1: 0x63EE58FB,
	Data2: 0x1268,
	Data3: 0x4835,
	Data4: [8]byte{0x86, 0xDA, 0xF0, 0x08, 0xCE, 0x62, 0xF0, 0xD6},
}

// IID_ID3D12Device is the interface ID for ID3D12Device.
// {189819F1-1DB6-4B57-BE54-1821339B85F7}
var IID_ID3D12Device = GUID{
	Data1: 0x189819F1,
	Data2: 0x1DB6,
	Data3: 0x4B57,
	Data4: [8]byte{0xBE, 0x54, 0x18, 0x21, 0x33, 0x9B, 0x85, 0xF7},
}

// IID_ID3D12Device1 is the interface ID for ID3D12Device1.
// {77ACCE80-638E-4E65-8895-C1F23386863E}
var IID_ID3D12Device1 = GUID{
	Data1: 0x77ACCE80,
	Data2: 0x638E,
	Data3: 0x4E65,
	Data4: [8]byte{0x88, 0x95, 0xC1, 0xF2, 0x33, 0x86, 0x86, 0x3E},
}

// IID_ID3D12Device2 is the interface ID for ID3D12Device2.
// {30BAA41E-B15B-475C-A0BB-1AF5C5B64328}
var IID_ID3D12Device2 = GUID{
	Data1: 0x30BAA41E,
	Data2: 0xB15B,
	Data3: 0x475C,
	Data4: [8]byte{0xA0, 0xBB, 0x1A, 0xF5, 0xC5, 0xB6, 0x43, 0x28},
}

// IID_ID3D12Device3 is the interface ID for ID3D12Device3.
// {81DADC15-2BAD-4392-93C5-101345C4AA98}
var IID_ID3D12Device3 = GUID{
	Data1: 0x81DADC15,
	Data2: 0x2BAD,
	Data3: 0x4392,
	Data4: [8]byte{0x93, 0xC5, 0x10, 0x13, 0x45, 0xC4, 0xAA, 0x98},
}

// IID_ID3D12Device4 is the interface ID for ID3D12Device4.
// {E865DF17-A9EE-46F9-A463-3098315AA2E5}
var IID_ID3D12Device4 = GUID{
	Data1: 0xE865DF17,
	Data2: 0xA9EE,
	Data3: 0x46F9,
	Data4: [8]byte{0xA4, 0x63, 0x30, 0x98, 0x31, 0x5A, 0xA2, 0xE5},
}

// IID_ID3D12Device5 is the interface ID for ID3D12Device5.
// {8B4F173B-2FEA-4B80-8F58-4307191AB95D}
var IID_ID3D12Device5 = GUID{
	Data1: 0x8B4F173B,
	Data2: 0x2FEA,
	Data3: 0x4B80,
	Data4: [8]byte{0x8F, 0x58, 0x43, 0x07, 0x19, 0x1A, 0xB9, 0x5D},
}

// IID_ID3D12CommandQueue is the interface ID for ID3D12CommandQueue.
// {0EC870A6-5D7E-4C22-8CFC-5BAAE07616ED}
var IID_ID3D12CommandQueue = GUID{
	Data1: 0x0EC870A6,
	Data2: 0x5D7E,
	Data3: 0x4C22,
	Data4: [8]byte{0x8C, 0xFC, 0x5B, 0xAA, 0xE0, 0x76, 0x16, 0xED},
}

// IID_ID3D12CommandAllocator is the interface ID for ID3D12CommandAllocator.
// {6102DEE4-AF59-4B09-B999-B44D73F09B24}
var IID_ID3D12CommandAllocator = GUID{
	Data1: 0x6102DEE4,
	Data2: 0xAF59,
	Data3: 0x4B09,
	Data4: [8]byte{0xB9, 0x99, 0xB4, 0x4D, 0x73, 0xF0, 0x9B, 0x24},
}

// IID_ID3D12CommandList is the interface ID for ID3D12CommandList.
// {7116D91C-E7E4-47CE-B8C6-EC8168F437E5}
var IID_ID3D12CommandList = GUID{
	Data1: 0x7116D91C,
	Data2: 0xE7E4,
	Data3: 0x47CE,
	Data4: [8]byte{0xB8, 0xC6, 0xEC, 0x81, 0x68, 0xF4, 0x37, 0xE5},
}

// IID_ID3D12GraphicsCommandList is the interface ID for ID3D12GraphicsCommandList.
// {5B160D0F-AC1B-4185-8BA8-B3AE42A5A455}
var IID_ID3D12GraphicsCommandList = GUID{
	Data1: 0x5B160D0F,
	Data2: 0xAC1B,
	Data3: 0x4185,
	Data4: [8]byte{0x8B, 0xA8, 0xB3, 0xAE, 0x42, 0xA5, 0xA4, 0x55},
}

// IID_ID3D12GraphicsCommandList1 is the interface ID for ID3D12GraphicsCommandList1.
// {553103FB-1FE7-4557-BB38-946D7D0E7CA7}
var IID_ID3D12GraphicsCommandList1 = GUID{
	Data1: 0x553103FB,
	Data2: 0x1FE7,
	Data3: 0x4557,
	Data4: [8]byte{0xBB, 0x38, 0x94, 0x6D, 0x7D, 0x0E, 0x7C, 0xA7},
}

// IID_ID3D12GraphicsCommandList2 is the interface ID for ID3D12GraphicsCommandList2.
// {38C3E585-FF17-412C-9150-4FC6F9D72A28}
var IID_ID3D12GraphicsCommandList2 = GUID{
	Data1: 0x38C3E585,
	Data2: 0xFF17,
	Data3: 0x412C,
	Data4: [8]byte{0x91, 0x50, 0x4F, 0xC6, 0xF9, 0xD7, 0x2A, 0x28},
}

// IID_ID3D12GraphicsCommandList3 is the interface ID for ID3D12GraphicsCommandList3.
// {6FDA83A7-B84C-4E38-9AC8-C7BD22016B3D}
var IID_ID3D12GraphicsCommandList3 = GUID{
	Data1: 0x6FDA83A7,
	Data2: 0xB84C,
	Data3: 0x4E38,
	Data4: [8]byte{0x9A, 0xC8, 0xC7, 0xBD, 0x22, 0x01, 0x6B, 0x3D},
}

// IID_ID3D12GraphicsCommandList4 is the interface ID for ID3D12GraphicsCommandList4.
// {8754318E-D3A9-4541-98CF-645B50DC4874}
var IID_ID3D12GraphicsCommandList4 = GUID{
	Data1: 0x8754318E,
	Data2: 0xD3A9,
	Data3: 0x4541,
	Data4: [8]byte{0x98, 0xCF, 0x64, 0x5B, 0x50, 0xDC, 0x48, 0x74},
}

// IID_ID3D12Fence is the interface ID for ID3D12Fence.
// {0A753DCF-C4D8-4B91-ADF6-BE5A60D95A76}
var IID_ID3D12Fence = GUID{
	Data1: 0x0A753DCF,
	Data2: 0xC4D8,
	Data3: 0x4B91,
	Data4: [8]byte{0xAD, 0xF6, 0xBE, 0x5A, 0x60, 0xD9, 0x5A, 0x76},
}

// IID_ID3D12Fence1 is the interface ID for ID3D12Fence1.
// {433685FE-E22B-4CA0-A8DB-B5B4F4DD0E4A}
var IID_ID3D12Fence1 = GUID{
	Data1: 0x433685FE,
	Data2: 0xE22B,
	Data3: 0x4CA0,
	Data4: [8]byte{0xA8, 0xDB, 0xB5, 0xB4, 0xF4, 0xDD, 0x0E, 0x4A},
}

// IID_ID3D12Resource is the interface ID for ID3D12Resource.
// {696442BE-A72E-4059-BC79-5B5C98040FAD}
var IID_ID3D12Resource = GUID{
	Data1: 0x696442BE,
	Data2: 0xA72E,
	Data3: 0x4059,
	Data4: [8]byte{0xBC, 0x79, 0x5B, 0x5C, 0x98, 0x04, 0x0F, 0xAD},
}

// IID_ID3D12Resource1 is the interface ID for ID3D12Resource1.
// {9D5E227A-4430-4161-88B3-3ECA6BB16E19}
var IID_ID3D12Resource1 = GUID{
	Data1: 0x9D5E227A,
	Data2: 0x4430,
	Data3: 0x4161,
	Data4: [8]byte{0x88, 0xB3, 0x3E, 0xCA, 0x6B, 0xB1, 0x6E, 0x19},
}

// IID_ID3D12DescriptorHeap is the interface ID for ID3D12DescriptorHeap.
// {8EFB471D-616C-4F49-90F7-127BB763FA51}
var IID_ID3D12DescriptorHeap = GUID{
	Data1: 0x8EFB471D,
	Data2: 0x616C,
	Data3: 0x4F49,
	Data4: [8]byte{0x90, 0xF7, 0x12, 0x7B, 0xB7, 0x63, 0xFA, 0x51},
}

// IID_ID3D12Heap is the interface ID for ID3D12Heap.
// {6B3B2502-6E51-45B3-90EE-9884265E8DF3}
var IID_ID3D12Heap = GUID{
	Data1: 0x6B3B2502,
	Data2: 0x6E51,
	Data3: 0x45B3,
	Data4: [8]byte{0x90, 0xEE, 0x98, 0x84, 0x26, 0x5E, 0x8D, 0xF3},
}

// IID_ID3D12Heap1 is the interface ID for ID3D12Heap1.
// {572F7389-2168-49E3-9693-D6DF5871BF6D}
var IID_ID3D12Heap1 = GUID{
	Data1: 0x572F7389,
	Data2: 0x2168,
	Data3: 0x49E3,
	Data4: [8]byte{0x96, 0x93, 0xD6, 0xDF, 0x58, 0x71, 0xBF, 0x6D},
}

// IID_ID3D12PipelineState is the interface ID for ID3D12PipelineState.
// {765A30F3-F624-4C6F-A828-ACE948622445}
var IID_ID3D12PipelineState = GUID{
	Data1: 0x765A30F3,
	Data2: 0xF624,
	Data3: 0x4C6F,
	Data4: [8]byte{0xA8, 0x28, 0xAC, 0xE9, 0x48, 0x62, 0x24, 0x45},
}

// IID_ID3D12RootSignature is the interface ID for ID3D12RootSignature.
// {C54A6B66-72DF-4EE8-8BE5-A946A1429214}
var IID_ID3D12RootSignature = GUID{
	Data1: 0xC54A6B66,
	Data2: 0x72DF,
	Data3: 0x4EE8,
	Data4: [8]byte{0x8B, 0xE5, 0xA9, 0x46, 0xA1, 0x42, 0x92, 0x14},
}

// IID_ID3D12QueryHeap is the interface ID for ID3D12QueryHeap.
// {0D9658AE-ED45-469E-A61D-970EC583CAB4}
var IID_ID3D12QueryHeap = GUID{
	Data1: 0x0D9658AE,
	Data2: 0xED45,
	Data3: 0x469E,
	Data4: [8]byte{0xA6, 0x1D, 0x97, 0x0E, 0xC5, 0x83, 0xCA, 0xB4},
}

// IID_ID3D12CommandSignature is the interface ID for ID3D12CommandSignature.
// {C36A797C-EC80-4F0A-8985-A7B2475082D1}
var IID_ID3D12CommandSignature = GUID{
	Data1: 0xC36A797C,
	Data2: 0xEC80,
	Data3: 0x4F0A,
	Data4: [8]byte{0x89, 0x85, 0xA7, 0xB2, 0x47, 0x50, 0x82, 0xD1},
}

// Debug Interface GUIDs

// IID_ID3D12Debug is the interface ID for ID3D12Debug.
// {344488B7-6846-474B-B989-F027448245E0}
var IID_ID3D12Debug = GUID{
	Data1: 0x344488B7,
	Data2: 0x6846,
	Data3: 0x474B,
	Data4: [8]byte{0xB9, 0x89, 0xF0, 0x27, 0x44, 0x82, 0x45, 0xE0},
}

// IID_ID3D12Debug1 is the interface ID for ID3D12Debug1.
// {AFFAA4CA-63FE-4D8E-B8AD-159000AF4304}
var IID_ID3D12Debug1 = GUID{
	Data1: 0xAFFAA4CA,
	Data2: 0x63FE,
	Data3: 0x4D8E,
	Data4: [8]byte{0xB8, 0xAD, 0x15, 0x90, 0x00, 0xAF, 0x43, 0x04},
}

// IID_ID3D12Debug2 is the interface ID for ID3D12Debug2.
// {93A665C4-A3B2-4E5D-B692-A26AE14E3374}
var IID_ID3D12Debug2 = GUID{
	Data1: 0x93A665C4,
	Data2: 0xA3B2,
	Data3: 0x4E5D,
	Data4: [8]byte{0xB6, 0x92, 0xA2, 0x6A, 0xE1, 0x4E, 0x33, 0x74},
}

// IID_ID3D12Debug3 is the interface ID for ID3D12Debug3.
// {5CF4E58F-F671-4FF1-A542-3686E3D153D1}
var IID_ID3D12Debug3 = GUID{
	Data1: 0x5CF4E58F,
	Data2: 0xF671,
	Data3: 0x4FF1,
	Data4: [8]byte{0xA5, 0x42, 0x36, 0x86, 0xE3, 0xD1, 0x53, 0xD1},
}

// IID_ID3D12InfoQueue is the interface ID for ID3D12InfoQueue.
// {0742A90B-C387-483F-B946-30A7E4E61458}
var IID_ID3D12InfoQueue = GUID{
	Data1: 0x0742A90B,
	Data2: 0xC387,
	Data3: 0x483F,
	Data4: [8]byte{0xB9, 0x46, 0x30, 0xA7, 0xE4, 0xE6, 0x14, 0x58},
}

// ID3DBlob GUID

// IID_ID3DBlob is the interface ID for ID3DBlob (ID3D10Blob).
// {8BA5FB08-5195-40E2-AC58-0D989C3A0102}
var IID_ID3DBlob = GUID{
	Data1: 0x8BA5FB08,
	Data2: 0x5195,
	Data3: 0x40E2,
	Data4: [8]byte{0xAC, 0x58, 0x0D, 0x98, 0x9C, 0x3A, 0x01, 0x02},
}
