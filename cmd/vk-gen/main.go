// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

// Command vk-gen generates Pure Go Vulkan bindings from vk.xml specification.
//
// Usage:
//
//	vk-gen -spec vk.xml -out ../hal/vulkan/vk/
//
//nolint:errcheck,gosec,gocritic,goconst,maintidx,funlen,gocyclo,cyclop,gocognit,nestif // code generator
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	specFile  = flag.String("spec", "vk.xml", "Path to vk.xml specification")
	outputDir = flag.String("out", "../hal/vulkan/vk/", "Output directory")
	// apiVersion reserved for future multi-API support (e.g., vulkansc)
)

func main() {
	flag.Parse()

	fmt.Printf("vk-gen: Generating Pure Go Vulkan bindings\n")
	fmt.Printf("  Spec: %s\n", *specFile)
	fmt.Printf("  Output: %s\n", *outputDir)

	// Parse vk.xml
	registry, err := parseSpec(*specFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing spec: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Types: %d\n", len(registry.Types.Types))
	fmt.Printf("  Enums: %d\n", len(registry.Enums))
	fmt.Printf("  Commands: %d\n", len(registry.Commands.Commands))

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output dir: %v\n", err)
		os.Exit(1)
	}

	// Generate files
	if err := generateConstants(registry, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating constants: %v\n", err)
		os.Exit(1)
	}

	if err := generateTypes(registry, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating types: %v\n", err)
		os.Exit(1)
	}

	if err := generateCommands(registry, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating commands: %v\n", err)
		os.Exit(1)
	}

	if err := generateLoader(registry, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating loader: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Generation complete!")
}

// Registry represents the root of vk.xml
type Registry struct {
	XMLName    xml.Name      `xml:"registry"`
	Types      TypesSection  `xml:"types"`
	Enums      []EnumsGroup  `xml:"enums"`
	Commands   CommandsGroup `xml:"commands"`
	Features   []Feature     `xml:"feature"`
	Extensions Extensions    `xml:"extensions"`
}

// Extensions contains all Vulkan extensions
type Extensions struct {
	Extensions []Extension `xml:"extension"`
}

// Extension represents a Vulkan extension
type Extension struct {
	Name      string             `xml:"name,attr"`
	Number    int                `xml:"number,attr"`
	Type      string             `xml:"type,attr"`
	Platform  string             `xml:"platform,attr"`
	Supported string             `xml:"supported,attr"`
	Requires  []ExtensionRequire `xml:"require"`
}

// ExtensionRequire contains required types and enums for an extension
type ExtensionRequire struct {
	Enums []ExtensionEnum `xml:"enum"`
	Types []ExtensionType `xml:"type"`
}

// ExtensionEnum is an enum value added by an extension
type ExtensionEnum struct {
	Name    string `xml:"name,attr"`
	Value   string `xml:"value,attr"`
	Offset  string `xml:"offset,attr"`
	Extends string `xml:"extends,attr"`
	Bitpos  string `xml:"bitpos,attr"`
	Dir     string `xml:"dir,attr"` // "-" for negative values
}

// ExtensionType is a type reference in an extension
type ExtensionType struct {
	Name string `xml:"name,attr"`
}

type TypesSection struct {
	Types []Type `xml:"type"`
}

type Type struct {
	Name      string   `xml:"name,attr"`
	Category  string   `xml:"category,attr"`
	Alias     string   `xml:"alias,attr"`
	Parent    string   `xml:"parent,attr"`
	Members   []Member `xml:"member"`
	InnerName string   `xml:"name"` // For types where name is element content
	Requires  string   `xml:"requires,attr"`
}

type Member struct {
	Name     string `xml:"name"`
	Type     string `xml:"type"`
	Enum     string `xml:"enum"` // Array size constant
	Values   string `xml:"values,attr"`
	Len      string `xml:"len,attr"`
	Optional string `xml:"optional,attr"`
	RawXML   string `xml:",innerxml"`
}

type EnumsGroup struct {
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Comment string `xml:"comment,attr"`
	Enums   []Enum `xml:"enum"`
}

type Enum struct {
	Name    string `xml:"name,attr"`
	Value   string `xml:"value,attr"`
	Bitpos  string `xml:"bitpos,attr"`
	Alias   string `xml:"alias,attr"`
	Comment string `xml:"comment,attr"`
}

type CommandsGroup struct {
	Commands []Command `xml:"command"`
}

type Command struct {
	Alias        string       `xml:"alias,attr"`
	Name         string       `xml:"name,attr"`
	Proto        CommandProto `xml:"proto"`
	Params       []Param      `xml:"param"`
	SuccessCodes string       `xml:"successcodes,attr"`
	ErrorCodes   string       `xml:"errorcodes,attr"`
}

type CommandProto struct {
	Type string `xml:"type"`
	Name string `xml:"name"`
}

type Param struct {
	Name     string `xml:"name"`
	Type     string `xml:"type"`
	Len      string `xml:"len,attr"`
	Optional string `xml:"optional,attr"`
	RawXML   string `xml:",innerxml"`
}

type Feature struct {
	API     string    `xml:"api,attr"`
	Name    string    `xml:"name,attr"`
	Number  string    `xml:"number,attr"`
	Require []Require `xml:"require"`
}

type Require struct {
	Types    []RequireType    `xml:"type"`
	Enums    []RequireEnum    `xml:"enum"`
	Commands []RequireCommand `xml:"command"`
}

type RequireType struct {
	Name string `xml:"name,attr"`
}

type RequireEnum struct {
	Name string `xml:"name,attr"`
}

type RequireCommand struct {
	Name string `xml:"name,attr"`
}

func parseSpec(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var registry Registry
	if err := xml.Unmarshal(data, &registry); err != nil {
		return nil, err
	}

	return &registry, nil
}

func generateConstants(registry *Registry, outDir string) error {
	f, err := os.Create(filepath.Join(outDir, "const_gen.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "// Code generated by vk-gen. DO NOT EDIT.")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "//go:build windows")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "package vk")
	fmt.Fprintln(f, "")

	// Generate VkResult and other enums
	for _, group := range registry.Enums {
		// Handle API Constants separately
		if group.Name == "API Constants" {
			if len(group.Enums) == 0 {
				continue
			}
			fmt.Fprintln(f, "// API Constants")
			fmt.Fprintln(f, "const (")
			for _, e := range group.Enums {
				if e.Alias != "" || e.Value == "" {
					continue
				}
				goName := vkToGoConst(e.Name)
				goValue := convertCValue(e.Value)
				fmt.Fprintf(f, "\t%s = %s\n", goName, goValue)
			}
			fmt.Fprintln(f, ")")
			fmt.Fprintln(f, "")
			continue
		}

		// Generate enum/bitmask types
		if group.Type == "enum" || group.Type == "bitmask" {
			typeName := vkToGoType(group.Name)
			// Use int64 for 64-bit flag types (ending in "2" like FlagBits2 or containing "64")
			baseType := "int32"
			if strings.HasSuffix(typeName, "2") || strings.Contains(typeName, "64") {
				baseType = "int64"
			}
			fmt.Fprintf(f, "// %s\n", group.Name)
			fmt.Fprintf(f, "type %s %s\n", typeName, baseType)
			fmt.Fprintln(f, "")

			// Only generate const block if there are values
			// (extension values will be generated separately)
			if len(group.Enums) > 0 {
				fmt.Fprintln(f, "const (")
				for _, e := range group.Enums {
					if e.Alias != "" {
						continue
					}
					goName := vkToGoConst(e.Name)
					if e.Bitpos != "" {
						fmt.Fprintf(f, "\t%s %s = 1 << %s\n", goName, typeName, e.Bitpos)
					} else if e.Value != "" {
						fmt.Fprintf(f, "\t%s %s = %s\n", goName, typeName, e.Value)
					}
				}
				fmt.Fprintln(f, ")")
				fmt.Fprintln(f, "")
			}
		}
	}

	// Collect all already-defined enum names to avoid duplicates
	definedEnums := make(map[string]bool)
	for _, group := range registry.Enums {
		for _, e := range group.Enums {
			if e.Alias == "" {
				definedEnums[vkToGoConst(e.Name)] = true
			}
		}
	}

	// Generate extension enum values grouped by the type they extend
	extensionEnums := collectExtensionEnums(registry)
	for extendsType, enums := range extensionEnums {
		goTypeName := vkToGoType(extendsType)

		// Filter out duplicates
		var uniqueEnums []ExtensionEnumValue
		seenNames := make(map[string]bool)
		for _, e := range enums {
			goName := vkToGoConst(e.Name)
			if !definedEnums[goName] && !seenNames[goName] {
				uniqueEnums = append(uniqueEnums, e)
				seenNames[goName] = true
			}
		}

		if len(uniqueEnums) == 0 {
			continue
		}

		fmt.Fprintf(f, "// %s extension values\n", extendsType)
		fmt.Fprintln(f, "const (")
		for _, e := range uniqueEnums {
			goName := vkToGoConst(e.Name)
			fmt.Fprintf(f, "\t%s %s = %d\n", goName, goTypeName, e.Value)
		}
		fmt.Fprintln(f, ")")
		fmt.Fprintln(f, "")
	}

	return nil
}

// ExtensionEnumValue holds a calculated extension enum value
type ExtensionEnumValue struct {
	Name  string
	Value int64
}

// collectExtensionEnums collects all extension enums and calculates their values
func collectExtensionEnums(registry *Registry) map[string][]ExtensionEnumValue {
	result := make(map[string][]ExtensionEnumValue)

	for _, ext := range registry.Extensions.Extensions {
		// Skip unsupported/disabled extensions
		if ext.Supported == "disabled" {
			continue
		}

		for _, req := range ext.Requires {
			for _, e := range req.Enums {
				if e.Extends == "" {
					continue // Not extending an existing enum
				}

				var value int64
				if e.Value != "" {
					// Direct value
					v, err := strconv.ParseInt(e.Value, 0, 64)
					if err != nil {
						continue
					}
					value = v
				} else if e.Offset != "" {
					// Calculate from extension number and offset
					// Formula: base + (ext_number - 1) * 1000 + offset
					// where base = 1000000000 for positive values
					offset, err := strconv.Atoi(e.Offset)
					if err != nil {
						continue
					}
					value = 1000000000 + int64(ext.Number-1)*1000 + int64(offset)
					if e.Dir == "-" {
						value = -value
					}
				} else if e.Bitpos != "" {
					// Bit position
					bitpos, err := strconv.Atoi(e.Bitpos)
					if err != nil {
						continue
					}
					value = 1 << bitpos
				} else {
					continue // No value, skip
				}

				result[e.Extends] = append(result[e.Extends], ExtensionEnumValue{
					Name:  e.Name,
					Value: value,
				})
			}
		}
	}

	return result
}

func generateTypes(registry *Registry, outDir string) error {
	f, err := os.Create(filepath.Join(outDir, "types_gen.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "// Code generated by vk-gen. DO NOT EDIT.")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "//go:build windows")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "package vk")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "import \"unsafe\"")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// Prevent unused import error")
	fmt.Fprintln(f, "var _ = unsafe.Sizeof(0)")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// Base types")
	fmt.Fprintln(f, "type (")
	fmt.Fprintln(f, "\tBool32        uint32")
	fmt.Fprintln(f, "\tDeviceSize    uint64")
	fmt.Fprintln(f, "\tDeviceAddress uint64")
	fmt.Fprintln(f, "\tFlags         uint32")
	fmt.Fprintln(f, "\tFlags64       uint64")
	fmt.Fprintln(f, "\tSampleMask    uint32")
	fmt.Fprintln(f, ")")
	fmt.Fprintln(f, "")

	// Bitmask types (Flags aliases)
	seenBitmasks := make(map[string]bool)
	bitmasks := []struct {
		name     string
		baseType string
	}{}
	for _, t := range registry.Types.Types {
		if t.Category == "bitmask" {
			name := t.Name
			if name == "" {
				name = t.InnerName
			}
			if name == "" || !strings.HasPrefix(name, "Vk") {
				continue
			}
			// Check if it's an alias
			if t.Alias != "" {
				continue
			}
			goName := vkToGoType(name)
			if seenBitmasks[goName] {
				continue
			}
			seenBitmasks[goName] = true
			// Determine base type (VkFlags or VkFlags64)
			baseType := "Flags"
			if strings.Contains(name, "64") || strings.Contains(t.Requires, "64") {
				baseType = "Flags64"
			}
			bitmasks = append(bitmasks, struct {
				name     string
				baseType string
			}{name, baseType})
		}
	}

	if len(bitmasks) > 0 {
		fmt.Fprintln(f, "// Bitmask types")
		fmt.Fprintln(f, "type (")
		for _, b := range bitmasks {
			goName := vkToGoType(b.name)
			fmt.Fprintf(f, "\t%s %s\n", goName, b.baseType)
		}
		fmt.Fprintln(f, ")")
		fmt.Fprintln(f, "")
	}

	// Platform-specific types as uintptr placeholders
	fmt.Fprintln(f, "// Platform-specific types (opaque pointers)")
	fmt.Fprintln(f, "type (")
	fmt.Fprintln(f, "\tANativeWindow          uintptr")
	fmt.Fprintln(f, "\tAHardwareBuffer        uintptr")
	fmt.Fprintln(f, "\tCAMetalLayer           uintptr")
	fmt.Fprintln(f, "\tWlDisplay              uintptr // wl_display")
	fmt.Fprintln(f, "\tWlSurface              uintptr // wl_surface")
	fmt.Fprintln(f, "\tXcbConnection          uintptr // xcb_connection_t")
	fmt.Fprintln(f, "\tXcbWindow              uint32  // xcb_window_t")
	fmt.Fprintln(f, "\tXcbVisualid            uint32  // xcb_visualid_t")
	fmt.Fprintln(f, "\tXlibDisplay            uintptr // Display*")
	fmt.Fprintln(f, "\tXlibWindow             uintptr // Window")
	fmt.Fprintln(f, "\tXlibVisualID           uintptr // VisualID")
	fmt.Fprintln(f, "\tZxBufferCollectionFUCHSIA uintptr")
	fmt.Fprintln(f, "\tGgpStreamDescriptor    uintptr")
	fmt.Fprintln(f, "\tGgpFrameToken          uintptr")
	fmt.Fprintln(f, "\tIDirectFB              uintptr")
	fmt.Fprintln(f, "\tIDirectFBSurface       uintptr")
	fmt.Fprintln(f, "\tScreenContext          uintptr // _screen_context")
	fmt.Fprintln(f, "\tScreenWindow           uintptr // _screen_window")
	fmt.Fprintln(f, "\tScreenBuffer           uintptr // _screen_buffer")
	fmt.Fprintln(f, "\tNvSciSyncAttrList      uintptr")
	fmt.Fprintln(f, "\tNvSciSyncObj           uintptr")
	fmt.Fprintln(f, "\tNvSciSyncFence         uintptr")
	fmt.Fprintln(f, "\tNvSciBufAttrList       uintptr")
	fmt.Fprintln(f, "\tNvSciBufObj            uintptr")
	fmt.Fprintln(f, "\tMTLDevice_id           uintptr")
	fmt.Fprintln(f, "\tMTLCommandQueue_id     uintptr")
	fmt.Fprintln(f, "\tMTLBuffer_id           uintptr")
	fmt.Fprintln(f, "\tMTLTexture_id          uintptr")
	fmt.Fprintln(f, "\tMTLSharedEvent_id      uintptr")
	fmt.Fprintln(f, "\tIOSurfaceRef           uintptr")
	fmt.Fprintln(f, ")")
	fmt.Fprintln(f, "")

	// Union types
	fmt.Fprintln(f, "// Union types (largest member size)")
	fmt.Fprintln(f, "type ClearValue [16]byte // Union: ClearColorValue or ClearDepthStencilValue")
	fmt.Fprintln(f, "type ClearColorValue [16]byte // Union: float32[4], int32[4], or uint32[4]")
	fmt.Fprintln(f, "type PerformanceValueDataINTEL [8]byte // Union: value32, value64, valueFloat, valueBool, valueString")
	fmt.Fprintln(f, "type PipelineExecutableStatisticValueKHR [8]byte // Union: b32, i64, u64, f64")
	fmt.Fprintln(f, "type PerformanceCounterResultKHR [8]byte // Union: int32, int64, uint32, uint64, float32, float64")
	fmt.Fprintln(f, "type DeviceOrHostAddressKHR uintptr // Union: deviceAddress or hostAddress")
	fmt.Fprintln(f, "type DeviceOrHostAddressConstKHR uintptr // Union: deviceAddress or hostAddress")
	fmt.Fprintln(f, "type AccelerationStructureGeometryDataKHR [64]byte // Union: triangles, aabbs, instances")
	fmt.Fprintln(f, "type AccelerationStructureMotionInstanceDataNV [144]byte // Union: static, matrix, srt instances")
	fmt.Fprintln(f, "type ClusterAccelerationStructureOpInputNV uintptr // Union: various cluster inputs")
	fmt.Fprintln(f, "type DescriptorDataEXT uintptr // Union: descriptor data variants")
	fmt.Fprintln(f, "")

	// Extension type aliases
	fmt.Fprintln(f, "// Extension type aliases")
	fmt.Fprintln(f, "type AccelerationStructureTypeNV = AccelerationStructureTypeKHR")
	fmt.Fprintln(f, "type BuildAccelerationStructureFlagsNV = BuildAccelerationStructureFlagsKHR")
	fmt.Fprintln(f, "type ComponentTypeNV = ComponentTypeKHR")
	fmt.Fprintln(f, "type ScopeNV = ScopeKHR")
	fmt.Fprintln(f, "type GeometryTypeNV = GeometryTypeKHR")
	fmt.Fprintln(f, "type GeometryFlagsNV = GeometryFlagsKHR")
	fmt.Fprintln(f, "type GeometryInstanceFlagsNV = GeometryInstanceFlagsKHR")
	fmt.Fprintln(f, "type CopyAccelerationStructureModeNV = CopyAccelerationStructureModeKHR")
	fmt.Fprintln(f, "type RayTracingShaderGroupTypeNV = RayTracingShaderGroupTypeKHR")
	fmt.Fprintln(f, "")

	// More union types
	fmt.Fprintln(f, "// More union types")
	fmt.Fprintln(f, "type IndirectExecutionSetInfoEXT uintptr")
	fmt.Fprintln(f, "type IndirectCommandsTokenDataEXT uintptr")
	fmt.Fprintln(f, "")

	// Video codec types (external headers - placeholders)
	fmt.Fprintln(f, "// Video codec types (external from vulkan_video_codec_*.h)")
	fmt.Fprintln(f, "type (")
	fmt.Fprintln(f, "\tStdVideoH264ProfileIdc             int32")
	fmt.Fprintln(f, "\tStdVideoH264LevelIdc               int32")
	fmt.Fprintln(f, "\tStdVideoH264ChromaFormatIdc        int32")
	fmt.Fprintln(f, "\tStdVideoH264PocType                int32")
	fmt.Fprintln(f, "\tStdVideoH264AspectRatioIdc         int32")
	fmt.Fprintln(f, "\tStdVideoH264WeightedBipredIdc      int32")
	fmt.Fprintln(f, "\tStdVideoH264ModificationOfPicNumsIdc int32")
	fmt.Fprintln(f, "\tStdVideoH264MemMgmtControlOp       int32")
	fmt.Fprintln(f, "\tStdVideoH264CabacInitIdc           int32")
	fmt.Fprintln(f, "\tStdVideoH264DisableDeblockingFilterIdc int32")
	fmt.Fprintln(f, "\tStdVideoH264SliceType              int32")
	fmt.Fprintln(f, "\tStdVideoH264PictureType            int32")
	fmt.Fprintln(f, "\tStdVideoH264NonVclNaluType         int32")
	fmt.Fprintln(f, "\tStdVideoH264SequenceParameterSet   [256]byte")
	fmt.Fprintln(f, "\tStdVideoH264PictureParameterSet    [128]byte")
	fmt.Fprintln(f, "\tStdVideoDecodeH264PictureInfo      [64]byte")
	fmt.Fprintln(f, "\tStdVideoDecodeH264ReferenceInfo    [32]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH264PictureInfo      [64]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH264ReferenceInfo    [32]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH264SliceHeader      [128]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH264ReferenceListsInfo [64]byte")
	fmt.Fprintln(f, "\tStdVideoH265ProfileIdc             int32")
	fmt.Fprintln(f, "\tStdVideoH265LevelIdc               int32")
	fmt.Fprintln(f, "\tStdVideoH265ChromaFormatIdc        int32")
	fmt.Fprintln(f, "\tStdVideoH265AspectRatioIdc         int32")
	fmt.Fprintln(f, "\tStdVideoH265SliceType              int32")
	fmt.Fprintln(f, "\tStdVideoH265PictureType            int32")
	fmt.Fprintln(f, "\tStdVideoH265VideoParameterSet      [256]byte")
	fmt.Fprintln(f, "\tStdVideoH265SequenceParameterSet   [512]byte")
	fmt.Fprintln(f, "\tStdVideoH265PictureParameterSet    [256]byte")
	fmt.Fprintln(f, "\tStdVideoDecodeH265PictureInfo      [64]byte")
	fmt.Fprintln(f, "\tStdVideoDecodeH265ReferenceInfo    [32]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH265PictureInfo      [64]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH265ReferenceInfo    [32]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH265SliceSegmentHeader [128]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeH265ReferenceListsInfo [64]byte")
	fmt.Fprintln(f, "\tStdVideoAV1Profile                 int32")
	fmt.Fprintln(f, "\tStdVideoAV1Level                   int32")
	fmt.Fprintln(f, "\tStdVideoAV1SequenceHeader          [256]byte")
	fmt.Fprintln(f, "\tStdVideoDecodeAV1PictureInfo       [128]byte")
	fmt.Fprintln(f, "\tStdVideoDecodeAV1ReferenceInfo     [64]byte")
	fmt.Fprintln(f, "\tStdVideoAV1TileInfo                [64]byte")
	fmt.Fprintln(f, "\tStdVideoAV1Quantization            [32]byte")
	fmt.Fprintln(f, "\tStdVideoAV1Segmentation            [64]byte")
	fmt.Fprintln(f, "\tStdVideoAV1LoopFilter              [32]byte")
	fmt.Fprintln(f, "\tStdVideoAV1CDEF                    [32]byte")
	fmt.Fprintln(f, "\tStdVideoAV1LoopRestoration         [16]byte")
	fmt.Fprintln(f, "\tStdVideoAV1GlobalMotion            [64]byte")
	fmt.Fprintln(f, "\tStdVideoAV1FilmGrain               [128]byte")
	// VP9 codec types
	fmt.Fprintln(f, "\tStdVideoVP9Profile                 int32")
	fmt.Fprintln(f, "\tStdVideoVP9Level                   int32")
	fmt.Fprintln(f, "\tStdVideoDecodeVP9PictureInfo       [128]byte")
	// AV1 encode types
	fmt.Fprintln(f, "\tStdVideoEncodeAV1DecoderModelInfo  [32]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeAV1OperatingPointInfo [64]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeAV1ReferenceInfo     [32]byte")
	fmt.Fprintln(f, "\tStdVideoEncodeAV1PictureInfo       [64]byte")
	fmt.Fprintln(f, ")")
	fmt.Fprintln(f, "")

	// More extension union and bitmask types
	fmt.Fprintln(f, "// More extension types")
	fmt.Fprintln(f, "type MemoryDecompressionMethodFlagsNV = Flags64")
	fmt.Fprintln(f, "type DeviceOrHostAddressConstAMDX uintptr")
	fmt.Fprintln(f, "type PipelineCreateFlags2KHR = Flags64")
	fmt.Fprintln(f, "")

	// OpenHarmony platform types
	fmt.Fprintln(f, "// OpenHarmony platform types")
	fmt.Fprintln(f, "type OHNativeWindow uintptr")
	fmt.Fprintln(f, "type OHBufferHandle uintptr")
	fmt.Fprintln(f, "type OH_NativeBuffer uintptr")
	fmt.Fprintln(f, "")

	// Handle types
	handles := []string{}
	for _, t := range registry.Types.Types {
		if t.Category == "handle" {
			name := t.Name
			if name == "" {
				name = t.InnerName
			}
			if name != "" && !strings.HasPrefix(name, "Vk") {
				continue
			}
			handles = append(handles, name)
		}
	}

	if len(handles) > 0 {
		fmt.Fprintln(f, "// Handles")
		fmt.Fprintln(f, "type (")
		for _, h := range handles {
			goName := vkToGoType(h)
			fmt.Fprintf(f, "\t%s uintptr\n", goName)
		}
		fmt.Fprintln(f, ")")
		fmt.Fprintln(f, "")
	}

	// Structs
	seenTypes := make(map[string]bool)
	// Skip types that are unions (defined manually above)
	skipTypes := map[string]bool{
		"ClearValue":                                true, // Union type - defined above
		"ClearColorValue":                           true, // Union type - defined above
		"PerformanceValueDataINTEL":                 true,
		"PipelineExecutableStatisticValueKHR":       true,
		"PerformanceCounterResultKHR":               true,
		"DeviceOrHostAddressKHR":                    true,
		"DeviceOrHostAddressConstKHR":               true,
		"AccelerationStructureGeometryDataKHR":      true,
		"AccelerationStructureMotionInstanceDataNV": true,
		"ClusterAccelerationStructureOpInputNV":     true,
		"DescriptorDataEXT":                         true,
		"IndirectExecutionSetInfoEXT":               true,
		"IndirectCommandsTokenDataEXT":              true,
	}
	for _, t := range registry.Types.Types {
		if t.Category != "struct" || t.Alias != "" {
			continue
		}
		if len(t.Members) == 0 {
			continue
		}

		name := t.Name
		goName := vkToGoType(name)

		if seenTypes[goName] || skipTypes[goName] {
			continue
		}
		seenTypes[goName] = true

		fmt.Fprintf(f, "// %s\n", name)
		fmt.Fprintf(f, "type %s struct {\n", goName)
		seenMembers := make(map[string]bool)
		for _, m := range t.Members {
			memberName := goFieldName(m.Name)
			// Skip duplicate members
			if seenMembers[memberName] {
				continue
			}
			seenMembers[memberName] = true
			memberType := vkToGoFieldType(m.Type, m.RawXML, m.Enum)
			fmt.Fprintf(f, "\t%s %s\n", memberName, memberType)
		}
		fmt.Fprintln(f, "}")
		fmt.Fprintln(f, "")
	}

	return nil
}

func generateCommands(registry *Registry, outDir string) error {
	f, err := os.Create(filepath.Join(outDir, "commands_gen.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "// Code generated by vk-gen. DO NOT EDIT.")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "//go:build windows")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "package vk")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "import (")
	fmt.Fprintln(f, "\t\"syscall\"")
	fmt.Fprintln(f, "\t\"unsafe\"")
	fmt.Fprintln(f, ")")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// Prevent unused import error")
	fmt.Fprintln(f, "var _ = unsafe.Sizeof(0)")
	fmt.Fprintln(f, "var _ = syscall.SyscallN")
	fmt.Fprintln(f, "")

	// Generate command function pointer struct
	fmt.Fprintln(f, "// Commands holds Vulkan function pointers.")
	fmt.Fprintln(f, "type Commands struct {")
	seen := make(map[string]bool)
	for _, cmd := range registry.Commands.Commands {
		if cmd.Alias != "" {
			continue
		}
		name := cmd.Proto.Name
		if name == "" {
			name = cmd.Name
		}
		if name == "" {
			continue
		}
		goName := strings.TrimPrefix(name, "vk")
		goName = strings.ToLower(goName[:1]) + goName[1:]
		if seen[goName] {
			continue
		}
		seen[goName] = true
		fmt.Fprintf(f, "\t%s uintptr\n", goName)
	}
	fmt.Fprintln(f, "}")
	fmt.Fprintln(f, "")

	return nil
}

func generateLoader(_ *Registry, outDir string) error {
	f, err := os.Create(filepath.Join(outDir, "loader_gen.go"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintln(f, "// Code generated by vk-gen. DO NOT EDIT.")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "//go:build windows")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "package vk")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "import (")
	fmt.Fprintln(f, "\t\"fmt\"")
	fmt.Fprintln(f, "\t\"syscall\"")
	fmt.Fprintln(f, "\t\"unsafe\"")
	fmt.Fprintln(f, ")")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "var vulkan *syscall.DLL")
	fmt.Fprintln(f, "var getInstanceProcAddr uintptr")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// Init loads the Vulkan library and core functions.")
	fmt.Fprintln(f, "func Init() error {")
	fmt.Fprintln(f, "\tvar err error")
	fmt.Fprintln(f, "\tvulkan, err = syscall.LoadDLL(\"vulkan-1.dll\")")
	fmt.Fprintln(f, "\tif err != nil {")
	fmt.Fprintln(f, "\t\treturn fmt.Errorf(\"failed to load vulkan-1.dll: %w\", err)")
	fmt.Fprintln(f, "\t}")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "\tproc, err := vulkan.FindProc(\"vkGetInstanceProcAddr\")")
	fmt.Fprintln(f, "\tif err != nil {")
	fmt.Fprintln(f, "\t\treturn fmt.Errorf(\"vkGetInstanceProcAddr not found: %w\", err)")
	fmt.Fprintln(f, "\t}")
	fmt.Fprintln(f, "\tgetInstanceProcAddr = proc.Addr()")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "\treturn nil")
	fmt.Fprintln(f, "}")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// GetInstanceProcAddr returns the address of a Vulkan function.")
	fmt.Fprintln(f, "func GetInstanceProcAddr(instance Instance, name string) uintptr {")
	fmt.Fprintln(f, "\tcname := append([]byte(name), 0)")
	fmt.Fprintln(f, "\tr, _, _ := syscall.SyscallN(getInstanceProcAddr,")
	fmt.Fprintln(f, "\t\tuintptr(instance),")
	fmt.Fprintln(f, "\t\tuintptr(unsafe.Pointer(&cname[0])))")
	fmt.Fprintln(f, "\treturn r")
	fmt.Fprintln(f, "}")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// LoadGlobalCommands loads Vulkan commands that don't require an instance.")
	fmt.Fprintln(f, "func (c *Commands) LoadGlobal() {")
	fmt.Fprintln(f, "\tc.createInstance = GetInstanceProcAddr(0, \"vkCreateInstance\")")
	fmt.Fprintln(f, "\tc.enumerateInstanceExtensionProperties = GetInstanceProcAddr(0, \"vkEnumerateInstanceExtensionProperties\")")
	fmt.Fprintln(f, "\tc.enumerateInstanceLayerProperties = GetInstanceProcAddr(0, \"vkEnumerateInstanceLayerProperties\")")
	fmt.Fprintln(f, "\tc.enumerateInstanceVersion = GetInstanceProcAddr(0, \"vkEnumerateInstanceVersion\")")
	fmt.Fprintln(f, "}")
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "// LoadInstance loads Vulkan commands that require an instance.")
	fmt.Fprintln(f, "func (c *Commands) LoadInstance(instance Instance) {")
	fmt.Fprintln(f, "\tc.destroyInstance = GetInstanceProcAddr(instance, \"vkDestroyInstance\")")
	fmt.Fprintln(f, "\tc.enumeratePhysicalDevices = GetInstanceProcAddr(instance, \"vkEnumeratePhysicalDevices\")")
	fmt.Fprintln(f, "\tc.getPhysicalDeviceProperties = GetInstanceProcAddr(instance, \"vkGetPhysicalDeviceProperties\")")
	fmt.Fprintln(f, "\tc.getPhysicalDeviceFeatures = GetInstanceProcAddr(instance, \"vkGetPhysicalDeviceFeatures\")")
	fmt.Fprintln(f, "\tc.getPhysicalDeviceQueueFamilyProperties = GetInstanceProcAddr(instance, \"vkGetPhysicalDeviceQueueFamilyProperties\")")
	fmt.Fprintln(f, "\tc.createDevice = GetInstanceProcAddr(instance, \"vkCreateDevice\")")
	fmt.Fprintln(f, "\t// TODO: Add more instance-level commands")
	fmt.Fprintln(f, "}")

	return nil
}

// convertCValue converts C-style constant values to Go
func convertCValue(value string) string {
	// Handle C-style suffixes and expressions
	value = strings.TrimSpace(value)

	// Float with F suffix: 1000.0F -> 1000.0
	if strings.HasSuffix(value, "F") || strings.HasSuffix(value, "f") {
		return strings.TrimSuffix(strings.TrimSuffix(value, "F"), "f")
	}

	// Unsigned long long: (~0ULL) -> ^uint64(0)
	if strings.Contains(value, "ULL") {
		value = strings.ReplaceAll(value, "(~0ULL)", "^uint64(0)")
		value = strings.ReplaceAll(value, "~0ULL", "^uint64(0)")
		value = strings.ReplaceAll(value, "ULL", "")
		return value
	}

	// Unsigned: (~0U), (~1U), (~2U) -> ^uint32(0), etc.
	if strings.Contains(value, "U)") || strings.HasSuffix(value, "U") {
		// (~0U) -> ^uint32(0)
		if strings.HasPrefix(value, "(~") && strings.HasSuffix(value, "U)") {
			inner := strings.TrimPrefix(value, "(~")
			inner = strings.TrimSuffix(inner, "U)")
			return "^uint32(" + inner + ")"
		}
		// Simple U suffix
		value = strings.TrimSuffix(value, "U")
		return value
	}

	return value
}

// Constants that conflict with type names - add Value suffix
var conflictingConstants = map[string]bool{
	"PipelineCacheHeaderVersionOne": true,
}

// vkToGoConst converts VK_CONSTANT_NAME to GoConstantName
func vkToGoConst(name string) string {
	// VK_SUCCESS -> Success
	// VK_ERROR_OUT_OF_HOST_MEMORY -> ErrorOutOfHostMemory
	name = strings.TrimPrefix(name, "VK_")
	parts := strings.Split(name, "_")
	var result strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		result.WriteString(strings.ToUpper(p[:1]))
		result.WriteString(strings.ToLower(p[1:]))
	}
	goName := result.String()
	// Check for conflicts with type names
	if conflictingConstants[goName] {
		return goName + "Value"
	}
	return goName
}

// vkToGoType converts VkTypeName to TypeName
func vkToGoType(name string) string {
	return strings.TrimPrefix(name, "Vk")
}

// goFieldName converts sType to SType (exported Go field name)
func goFieldName(name string) string {
	if name == "" {
		return ""
	}
	// Handle special cases
	switch name {
	case "sType":
		return "SType"
	case "pNext":
		return "PNext"
	}
	// Capitalize first letter
	return strings.ToUpper(name[:1]) + name[1:]
}

// vkToGoFieldType converts Vulkan type to Go type
func vkToGoFieldType(vkType string, rawXML string, enumSize string) string {
	// Check for pointer
	isPointer := strings.Contains(rawXML, "*")
	isDoublePointer := strings.Contains(rawXML, "**") || strings.Contains(rawXML, "* const*")

	// Check for array: [N] or [ENUM_CONSTANT]
	isArray := strings.Contains(rawXML, "[")

	baseType := vkToGoBaseType(vkType)

	if isDoublePointer {
		return "uintptr" // char**, void**, etc.
	}
	if isPointer {
		if baseType == "byte" || vkType == "char" {
			return "uintptr" // char* as uintptr for strings
		}
		return "*" + baseType
	}

	// Handle arrays
	if isArray {
		// Extract array size from rawXML like "[VK_MAX_PHYSICAL_DEVICE_NAME_SIZE]"
		// or from enumSize field
		arraySize := ""
		if enumSize != "" {
			// Convert VK_MAX_PHYSICAL_DEVICE_NAME_SIZE to the constant value
			arraySize = convertEnumToSize(enumSize)
		} else {
			// Try to extract from rawXML
			start := strings.Index(rawXML, "[")
			end := strings.Index(rawXML, "]")
			if start >= 0 && end > start {
				sizeStr := rawXML[start+1 : end]
				// Remove any nested tags like <enum>
				sizeStr = strings.TrimPrefix(sizeStr, "<enum>")
				sizeStr = strings.TrimSuffix(sizeStr, "</enum>")
				arraySize = convertEnumToSize(sizeStr)
			}
		}
		if arraySize != "" {
			return "[" + arraySize + "]" + baseType
		}
	}

	return baseType
}

// convertEnumToSize converts VK_* constants to their numeric values
func convertEnumToSize(enumName string) string {
	// Map of known array size constants
	sizeMap := map[string]string{
		"VK_MAX_PHYSICAL_DEVICE_NAME_SIZE":          "256",
		"VK_UUID_SIZE":                              "16",
		"VK_LUID_SIZE":                              "8",
		"VK_MAX_EXTENSION_NAME_SIZE":                "256",
		"VK_MAX_DESCRIPTION_SIZE":                   "256",
		"VK_MAX_MEMORY_TYPES":                       "32",
		"VK_MAX_MEMORY_HEAPS":                       "16",
		"VK_MAX_DRIVER_NAME_SIZE":                   "256",
		"VK_MAX_DRIVER_INFO_SIZE":                   "256",
		"VK_MAX_DEVICE_GROUP_SIZE":                  "32",
		"VK_MAX_GLOBAL_PRIORITY_SIZE_KHR":           "16",
		"VK_MAX_GLOBAL_PRIORITY_SIZE_EXT":           "16",
		"VK_MAX_SHADER_MODULE_IDENTIFIER_SIZE_EXT":  "32",
		"VK_MAX_VIDEO_AV1_REFERENCES_PER_FRAME_KHR": "7",
		// Matrix sizes
		"3":  "3",
		"4":  "4",
		"2":  "2",
		"12": "12",
	}

	if size, ok := sizeMap[enumName]; ok {
		return size
	}

	// If it's a plain number, return as is
	if _, err := strconv.Atoi(enumName); err == nil {
		return enumName
	}

	// Unknown constant - use a default or return empty
	return ""
}

func vkToGoBaseType(vkType string) string {
	switch vkType {
	case "void":
		return "uintptr"
	case "char":
		return "byte"
	case "float":
		return "float32"
	case "double":
		return "float64"
	case "int8_t":
		return "int8"
	case "uint8_t":
		return "uint8"
	case "int16_t":
		return "int16"
	case "uint16_t":
		return "uint16"
	case "int32_t":
		return "int32"
	case "uint32_t":
		return "uint32"
	case "int64_t":
		return "int64"
	case "uint64_t":
		return "uint64"
	case "size_t":
		return "uintptr"
	case "VkBool32":
		return "Bool32"
	case "VkDeviceSize":
		return "DeviceSize"
	case "VkDeviceAddress":
		return "DeviceAddress"
	case "VkFlags":
		return "Flags"
	case "VkFlags64":
		return "Flags64"
	case "VkSampleMask":
		return "SampleMask"
	// Platform-specific types
	case "ANativeWindow":
		return "ANativeWindow"
	case "AHardwareBuffer":
		return "AHardwareBuffer"
	case "CAMetalLayer":
		return "CAMetalLayer"
	case "wl_display":
		return "WlDisplay"
	case "wl_surface":
		return "WlSurface"
	case "xcb_connection_t":
		return "XcbConnection"
	case "xcb_window_t":
		return "XcbWindow"
	case "xcb_visualid_t":
		return "XcbVisualid"
	case "Display":
		return "XlibDisplay"
	case "Window":
		return "XlibWindow"
	case "VisualID":
		return "XlibVisualID"
	case "zx_handle_t":
		return "uint32"
	case "GgpStreamDescriptor":
		return "GgpStreamDescriptor"
	case "GgpFrameToken":
		return "GgpFrameToken"
	case "IDirectFB":
		return "IDirectFB"
	case "IDirectFBSurface":
		return "IDirectFBSurface"
	case "_screen_context":
		return "ScreenContext"
	case "_screen_window":
		return "ScreenWindow"
	case "_screen_buffer":
		return "ScreenBuffer"
	case "NvSciSyncAttrList":
		return "NvSciSyncAttrList"
	case "NvSciSyncObj":
		return "NvSciSyncObj"
	case "NvSciSyncFence":
		return "NvSciSyncFence"
	case "NvSciBufAttrList":
		return "NvSciBufAttrList"
	case "NvSciBufObj":
		return "NvSciBufObj"
	case "MTLDevice_id":
		return "MTLDevice_id"
	case "MTLCommandQueue_id":
		return "MTLCommandQueue_id"
	case "MTLBuffer_id":
		return "MTLBuffer_id"
	case "MTLTexture_id":
		return "MTLTexture_id"
	case "MTLSharedEvent_id":
		return "MTLSharedEvent_id"
	case "IOSurfaceRef":
		return "IOSurfaceRef"
	case "HINSTANCE":
		return "uintptr"
	case "HWND":
		return "uintptr"
	case "HMONITOR":
		return "uintptr"
	case "HANDLE":
		return "uintptr"
	case "DWORD":
		return "uint32"
	case "LPCWSTR":
		return "uintptr"
	case "SECURITY_ATTRIBUTES":
		return "uintptr"
	case "RROutput":
		return "uintptr"
	default:
		if strings.HasPrefix(vkType, "Vk") {
			return strings.TrimPrefix(vkType, "Vk")
		}
		if strings.HasPrefix(vkType, "PFN_") {
			return "uintptr" // Function pointers
		}
		return vkType
	}
}
