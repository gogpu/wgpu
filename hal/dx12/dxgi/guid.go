// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows

package dxgi

// GUID represents a Windows GUID (Globally Unique Identifier).
// Layout must match Windows GUID structure exactly.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// DXGI Interface GUIDs
// Reference: https://learn.microsoft.com/en-us/windows/win32/api/dxgi/

// IID_IDXGIObject is the interface ID for IDXGIObject.
// {AEC22FB8-76F3-4639-9BE0-28EB43A67A2E}
var IID_IDXGIObject = GUID{
	Data1: 0xAEC22FB8,
	Data2: 0x76F3,
	Data3: 0x4639,
	Data4: [8]byte{0x9B, 0xE0, 0x28, 0xEB, 0x43, 0xA6, 0x7A, 0x2E},
}

// IID_IDXGIDeviceSubObject is the interface ID for IDXGIDeviceSubObject.
// {3D3E0379-F9DE-4D58-BB6C-18D62992F1A6}
var IID_IDXGIDeviceSubObject = GUID{
	Data1: 0x3D3E0379,
	Data2: 0xF9DE,
	Data3: 0x4D58,
	Data4: [8]byte{0xBB, 0x6C, 0x18, 0xD6, 0x29, 0x92, 0xF1, 0xA6},
}

// IID_IDXGIFactory is the interface ID for IDXGIFactory.
// {7B7166EC-21C7-44AE-B21A-C9AE321AE369}
var IID_IDXGIFactory = GUID{
	Data1: 0x7B7166EC,
	Data2: 0x21C7,
	Data3: 0x44AE,
	Data4: [8]byte{0xB2, 0x1A, 0xC9, 0xAE, 0x32, 0x1A, 0xE3, 0x69},
}

// IID_IDXGIFactory1 is the interface ID for IDXGIFactory1.
// {770AAE78-F26F-4DBA-A829-253C83D1B387}
var IID_IDXGIFactory1 = GUID{
	Data1: 0x770AAE78,
	Data2: 0xF26F,
	Data3: 0x4DBA,
	Data4: [8]byte{0xA8, 0x29, 0x25, 0x3C, 0x83, 0xD1, 0xB3, 0x87},
}

// IID_IDXGIFactory2 is the interface ID for IDXGIFactory2.
// {50C83A1C-E072-4C48-87B0-3630FA36A6D0}
var IID_IDXGIFactory2 = GUID{
	Data1: 0x50C83A1C,
	Data2: 0xE072,
	Data3: 0x4C48,
	Data4: [8]byte{0x87, 0xB0, 0x36, 0x30, 0xFA, 0x36, 0xA6, 0xD0},
}

// IID_IDXGIFactory3 is the interface ID for IDXGIFactory3.
// {25483823-CD46-4C7D-86CA-47AA95B837BD}
var IID_IDXGIFactory3 = GUID{
	Data1: 0x25483823,
	Data2: 0xCD46,
	Data3: 0x4C7D,
	Data4: [8]byte{0x86, 0xCA, 0x47, 0xAA, 0x95, 0xB8, 0x37, 0xBD},
}

// IID_IDXGIFactory4 is the interface ID for IDXGIFactory4.
// {1BC6EA02-EF36-464F-BF0C-21CA39E5168A}
var IID_IDXGIFactory4 = GUID{
	Data1: 0x1BC6EA02,
	Data2: 0xEF36,
	Data3: 0x464F,
	Data4: [8]byte{0xBF, 0x0C, 0x21, 0xCA, 0x39, 0xE5, 0x16, 0x8A},
}

// IID_IDXGIFactory5 is the interface ID for IDXGIFactory5.
// {7632E1F5-EE65-4DCA-87FD-84CD75F8838D}
var IID_IDXGIFactory5 = GUID{
	Data1: 0x7632E1F5,
	Data2: 0xEE65,
	Data3: 0x4DCA,
	Data4: [8]byte{0x87, 0xFD, 0x84, 0xCD, 0x75, 0xF8, 0x83, 0x8D},
}

// IID_IDXGIFactory6 is the interface ID for IDXGIFactory6.
// {C1B6694F-FF09-44A9-B03C-77900A0A1D17}
var IID_IDXGIFactory6 = GUID{
	Data1: 0xC1B6694F,
	Data2: 0xFF09,
	Data3: 0x44A9,
	Data4: [8]byte{0xB0, 0x3C, 0x77, 0x90, 0x0A, 0x0A, 0x1D, 0x17},
}

// IID_IDXGIFactory7 is the interface ID for IDXGIFactory7.
// {A4966EED-76DB-44DA-84C1-EE9A7AFB20A8}
var IID_IDXGIFactory7 = GUID{
	Data1: 0xA4966EED,
	Data2: 0x76DB,
	Data3: 0x44DA,
	Data4: [8]byte{0x84, 0xC1, 0xEE, 0x9A, 0x7A, 0xFB, 0x20, 0xA8},
}

// IID_IDXGIAdapter is the interface ID for IDXGIAdapter.
// {2411E7E1-12AC-4CCF-BD14-9798E8534DC0}
var IID_IDXGIAdapter = GUID{
	Data1: 0x2411E7E1,
	Data2: 0x12AC,
	Data3: 0x4CCF,
	Data4: [8]byte{0xBD, 0x14, 0x97, 0x98, 0xE8, 0x53, 0x4D, 0xC0},
}

// IID_IDXGIAdapter1 is the interface ID for IDXGIAdapter1.
// {29038F61-3839-4626-91FD-086879011A05}
var IID_IDXGIAdapter1 = GUID{
	Data1: 0x29038F61,
	Data2: 0x3839,
	Data3: 0x4626,
	Data4: [8]byte{0x91, 0xFD, 0x08, 0x68, 0x79, 0x01, 0x1A, 0x05},
}

// IID_IDXGIAdapter2 is the interface ID for IDXGIAdapter2.
// {0AA1AE0A-FA0E-4B84-8644-E05FF8E5ACB5}
var IID_IDXGIAdapter2 = GUID{
	Data1: 0x0AA1AE0A,
	Data2: 0xFA0E,
	Data3: 0x4B84,
	Data4: [8]byte{0x86, 0x44, 0xE0, 0x5F, 0xF8, 0xE5, 0xAC, 0xB5},
}

// IID_IDXGIAdapter3 is the interface ID for IDXGIAdapter3.
// {645967A4-1392-4310-A798-8053CE3E93FD}
var IID_IDXGIAdapter3 = GUID{
	Data1: 0x645967A4,
	Data2: 0x1392,
	Data3: 0x4310,
	Data4: [8]byte{0xA7, 0x98, 0x80, 0x53, 0xCE, 0x3E, 0x93, 0xFD},
}

// IID_IDXGIAdapter4 is the interface ID for IDXGIAdapter4.
// {3C8D99D1-4FBF-4181-A82C-AF66BF7BD24E}
var IID_IDXGIAdapter4 = GUID{
	Data1: 0x3C8D99D1,
	Data2: 0x4FBF,
	Data3: 0x4181,
	Data4: [8]byte{0xA8, 0x2C, 0xAF, 0x66, 0xBF, 0x7B, 0xD2, 0x4E},
}

// IID_IDXGIOutput is the interface ID for IDXGIOutput.
// {AE02EEDB-C735-4690-8D52-5A8DC20213AA}
var IID_IDXGIOutput = GUID{
	Data1: 0xAE02EEDB,
	Data2: 0xC735,
	Data3: 0x4690,
	Data4: [8]byte{0x8D, 0x52, 0x5A, 0x8D, 0xC2, 0x02, 0x13, 0xAA},
}

// IID_IDXGIOutput1 is the interface ID for IDXGIOutput1.
// {00CDDEA8-939B-4B83-A340-A685226666CC}
var IID_IDXGIOutput1 = GUID{
	Data1: 0x00CDDEA8,
	Data2: 0x939B,
	Data3: 0x4B83,
	Data4: [8]byte{0xA3, 0x40, 0xA6, 0x85, 0x22, 0x66, 0x66, 0xCC},
}

// IID_IDXGIOutput2 is the interface ID for IDXGIOutput2.
// {595E39D1-2724-4663-99B1-DA969DE28364}
var IID_IDXGIOutput2 = GUID{
	Data1: 0x595E39D1,
	Data2: 0x2724,
	Data3: 0x4663,
	Data4: [8]byte{0x99, 0xB1, 0xDA, 0x96, 0x9D, 0xE2, 0x83, 0x64},
}

// IID_IDXGIOutput3 is the interface ID for IDXGIOutput3.
// {8A6BB301-7E7E-41F4-A8E0-5B32F7F99B18}
var IID_IDXGIOutput3 = GUID{
	Data1: 0x8A6BB301,
	Data2: 0x7E7E,
	Data3: 0x41F4,
	Data4: [8]byte{0xA8, 0xE0, 0x5B, 0x32, 0xF7, 0xF9, 0x9B, 0x18},
}

// IID_IDXGIOutput4 is the interface ID for IDXGIOutput4.
// {DC7DCA35-2196-414D-9F53-617884032A60}
var IID_IDXGIOutput4 = GUID{
	Data1: 0xDC7DCA35,
	Data2: 0x2196,
	Data3: 0x414D,
	Data4: [8]byte{0x9F, 0x53, 0x61, 0x78, 0x84, 0x03, 0x2A, 0x60},
}

// IID_IDXGISwapChain is the interface ID for IDXGISwapChain.
// {310D36A0-D2E7-4C0A-AA04-6A9D23B8886A}
var IID_IDXGISwapChain = GUID{
	Data1: 0x310D36A0,
	Data2: 0xD2E7,
	Data3: 0x4C0A,
	Data4: [8]byte{0xAA, 0x04, 0x6A, 0x9D, 0x23, 0xB8, 0x88, 0x6A},
}

// IID_IDXGISwapChain1 is the interface ID for IDXGISwapChain1.
// {790A45F7-0D42-4876-983A-0A55CFE6F4AA}
var IID_IDXGISwapChain1 = GUID{
	Data1: 0x790A45F7,
	Data2: 0x0D42,
	Data3: 0x4876,
	Data4: [8]byte{0x98, 0x3A, 0x0A, 0x55, 0xCF, 0xE6, 0xF4, 0xAA},
}

// IID_IDXGISwapChain2 is the interface ID for IDXGISwapChain2.
// {A8BE2AC4-199F-4946-B331-79599FB98DE7}
var IID_IDXGISwapChain2 = GUID{
	Data1: 0xA8BE2AC4,
	Data2: 0x199F,
	Data3: 0x4946,
	Data4: [8]byte{0xB3, 0x31, 0x79, 0x59, 0x9F, 0xB9, 0x8D, 0xE7},
}

// IID_IDXGISwapChain3 is the interface ID for IDXGISwapChain3.
// {94D99BDB-F1F8-4AB0-B236-7DA0170EDAB1}
var IID_IDXGISwapChain3 = GUID{
	Data1: 0x94D99BDB,
	Data2: 0xF1F8,
	Data3: 0x4AB0,
	Data4: [8]byte{0xB2, 0x36, 0x7D, 0xA0, 0x17, 0x0E, 0xDA, 0xB1},
}

// IID_IDXGISwapChain4 is the interface ID for IDXGISwapChain4.
// {3D585D5A-BD4A-489E-B1F4-3DBCB6452FFB}
var IID_IDXGISwapChain4 = GUID{
	Data1: 0x3D585D5A,
	Data2: 0xBD4A,
	Data3: 0x489E,
	Data4: [8]byte{0xB1, 0xF4, 0x3D, 0xBC, 0xB6, 0x45, 0x2F, 0xFB},
}

// IID_IDXGIDevice is the interface ID for IDXGIDevice.
// {54EC77FA-1377-44E6-8C32-88FD5F44C84C}
var IID_IDXGIDevice = GUID{
	Data1: 0x54EC77FA,
	Data2: 0x1377,
	Data3: 0x44E6,
	Data4: [8]byte{0x8C, 0x32, 0x88, 0xFD, 0x5F, 0x44, 0xC8, 0x4C},
}

// IID_IDXGISurface is the interface ID for IDXGISurface.
// {CAFCB56C-6AC3-4889-BF47-9E23BBD260EC}
var IID_IDXGISurface = GUID{
	Data1: 0xCAFCB56C,
	Data2: 0x6AC3,
	Data3: 0x4889,
	Data4: [8]byte{0xBF, 0x47, 0x9E, 0x23, 0xBB, 0xD2, 0x60, 0xEC},
}

// IID_IDXGIResource is the interface ID for IDXGIResource.
// {035F3AB4-482E-4E50-B41F-8A7F8BD8960B}
var IID_IDXGIResource = GUID{
	Data1: 0x035F3AB4,
	Data2: 0x482E,
	Data3: 0x4E50,
	Data4: [8]byte{0xB4, 0x1F, 0x8A, 0x7F, 0x8B, 0xD8, 0x96, 0x0B},
}
