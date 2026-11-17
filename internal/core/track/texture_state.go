//go:build !(js && wasm)

package track

// textureState is the usage of every subresource of one texture: usage for
// all of them while it is whole, and otherwise each its own in subresources.
//
// Reference: wgpu-core track/texture.rs TextureStateSet (simple and complex)
type textureState struct {
	usage TextureUses
	// subresources holds the usage of each subresource once their usages
	// differ; nil or empty while the state is whole. Its storage is kept
	// for the next split.
	subresources *subresourceUsageTable
}

// isWhole returns true if every subresource has the same usage.
func (s *textureState) isWhole() bool {
	return s.subresources == nil || len(s.subresources.usages) == 0
}

// wholeUsage returns the usage every subresource shares, or TextureUsesNone
// if they differ.
func (s *textureState) wholeUsage() TextureUses {
	if s.isWhole() {
		return s.usage
	}
	return TextureUsesNone
}

// counts returns the mip level and array layer counts of a split state.
func (s *textureState) counts() (mipLevelCount, arrayLayerCount uint32) {
	arrayLayerCount = s.subresources.arrayLayerCount
	return uint32(len(s.subresources.usages)) / arrayLayerCount, arrayLayerCount //nolint:gosec // a texture's subresource count fits uint32
}

// at returns the usage of one subresource of a split state.
func (s *textureState) at(mipLevel, arrayLayer uint32) *TextureUses {
	return &s.subresources.usages[mipLevel*s.subresources.arrayLayerCount+arrayLayer]
}

// subresourceUsage returns the usage of one subresource.
func (s *textureState) subresourceUsage(mipLevel, arrayLayer uint32) TextureUses {
	if s.isWhole() {
		return s.usage
	}
	if mipLevelCount, arrayLayerCount := s.counts(); mipLevel >= mipLevelCount || arrayLayer >= arrayLayerCount {
		return TextureUsesNone
	}
	return *s.at(mipLevel, arrayLayer)
}

// reset gives every subresource usage.
func (s *textureState) reset(usage TextureUses) {
	s.usage = usage
	if s.subresources != nil {
		s.subresources.usages = s.subresources.usages[:0]
	}
}

// copyFrom makes s a copy of other.
func (s *textureState) copyFrom(other *textureState) {
	s.reset(other.usage)
	if other.isWhole() {
		return
	}
	if s.subresources == nil {
		s.subresources = &subresourceUsageTable{}
	}
	s.subresources.arrayLayerCount = other.subresources.arrayLayerCount
	s.subresources.usages = append(s.subresources.usages, other.subresources.usages...)
}

// split gives every subresource of a texture of mipLevelCount levels and
// arrayLayerCount layers a usage of its own, each the usage they share. It
// does nothing to a state already split.
func (s *textureState) split(mipLevelCount, arrayLayerCount uint32) {
	if !s.isWhole() {
		return
	}
	if s.subresources == nil {
		s.subresources = &subresourceUsageTable{}
	}
	s.subresources.arrayLayerCount = arrayLayerCount
	for range mipLevelCount * arrayLayerCount {
		s.subresources.usages = append(s.subresources.usages, s.usage)
	}
}

// collapse makes a split state whole again when every subresource has the
// same usage.
func (s *textureState) collapse() {
	if s.isWhole() {
		return
	}
	firstUsage := s.subresources.usages[0]
	for _, usage := range s.subresources.usages[1:] {
		if usage != firstUsage {
			return
		}
	}
	s.reset(firstUsage)
}

// merge adds usage of the subresources selector selects, as a usage scope
// does. Returns an error, and changes nothing, if one of them already has an
// incompatible usage. The error selects the subresources the conflict
// covers, as TextureUsageConflictError.Selector says.
//
// Reference: wgpu-core track/texture.rs TextureUsageScope::merge_single
func (s *textureState) merge(index TrackerIndex, selector TextureSelector, usage TextureUses) error {
	if s.isWhole() && s.usage|usage == s.usage {
		return nil
	}
	mipLevelCount, arrayLayerCount := subresourceCounts(selector, s)
	if mipLevelCount == 0 {
		combined := s.usage | usage
		if !combined.IsCompatible(combined) {
			return &TextureUsageConflictError{Index: index, Existing: s.usage, New: usage}
		}
		s.usage = combined
		return nil
	}
	mipStart, mipEnd, layerStart, layerEnd := selector.bounds(mipLevelCount, arrayLayerCount)
	for mipLevel := mipStart; mipLevel < mipEnd; mipLevel++ {
		for arrayLayer := layerStart; arrayLayer < layerEnd; arrayLayer++ {
			existing := s.subresourceUsage(mipLevel, arrayLayer)
			if combined := existing | usage; !combined.IsCompatible(combined) {
				conflicting := TextureSelector{
					TextureMipLevelCount: mipLevelCount, TextureArrayLayerCount: arrayLayerCount,
					BaseMipLevel: mipStart, MipLevelCount: mipEnd - mipStart,
					BaseArrayLayer: layerStart, ArrayLayerCount: layerEnd - layerStart,
				}
				if !s.isWhole() {
					conflicting = s.layersSharingUsage(mipLevelCount, arrayLayerCount, mipLevel, arrayLayer, layerEnd)
				}
				return &TextureUsageConflictError{Index: index, Selector: conflicting, Existing: existing, New: usage}
			}
		}
	}
	s.split(mipLevelCount, arrayLayerCount)
	for mipLevel := mipStart; mipLevel < mipEnd; mipLevel++ {
		for arrayLayer := layerStart; arrayLayer < layerEnd; arrayLayer++ {
			*s.at(mipLevel, arrayLayer) |= usage
		}
	}
	s.collapse()
	return nil
}

// set moves the subresources selector selects to usage, as a device does
// outside any command buffer, adding the barriers from their current states
// to transitions.
//
// Reference: wgpu-core track/texture.rs DeviceTextureTracker::set_single
func (s *textureState) set(transitions *textureTransitionList, index TrackerIndex, selector TextureSelector, usage TextureUses) {
	mipLevelCount, arrayLayerCount := subresourceCounts(selector, s)
	whole := s.isWhole()
	if whole {
		// The selected subresources share one state: one barrier over them.
		if !SkipBarrier(s.usage, usage) {
			transitions.addSelected(index, selector, s.usage, usage)
		}
		if mipLevelCount == 0 || s.usage == usage {
			s.usage = usage
			return
		}
		s.split(mipLevelCount, arrayLayerCount)
	}
	mipStart, mipEnd, layerStart, layerEnd := selector.bounds(mipLevelCount, arrayLayerCount)
	for mipLevel := mipStart; mipLevel < mipEnd; mipLevel++ {
		for arrayLayer := layerStart; arrayLayer < layerEnd; arrayLayer++ {
			subresource := s.at(mipLevel, arrayLayer)
			if !whole && !SkipBarrier(*subresource, usage) {
				transitions.addSubresource(index, mipLevelCount, arrayLayerCount, mipLevel, arrayLayer, *subresource, usage)
			}
			*subresource = usage
		}
	}
	s.collapse()
}

// advance applies a command buffer's use of a texture to the device's state
// of it: each subresource the command buffer uses moves from its own state
// to the one the command buffer first uses it in, adding the barrier to
// transitions, then takes the one the command buffer leaves it in.
// Subresources start leaves at TextureUsesNone are not used.
//
// Reference: wgpu-core track/texture.rs DeviceTextureTracker::set_from_tracker_and_drain_transitions
func (s *textureState) advance(transitions *textureTransitionList, index TrackerIndex, start, end *textureState) {
	mipLevelCount, arrayLayerCount := subresourceCounts(TextureSelector{}, s, start, end)
	if mipLevelCount == 0 {
		if start.usage == TextureUsesNone {
			return
		}
		if !SkipBarrier(s.usage, start.usage) {
			transitions.addSelected(index, TextureSelector{}, s.usage, start.usage)
		}
		s.usage = end.usage
		return
	}
	s.split(mipLevelCount, arrayLayerCount)
	for mipLevel := range mipLevelCount {
		for arrayLayer := range arrayLayerCount {
			firstUsage := start.subresourceUsage(mipLevel, arrayLayer)
			if firstUsage == TextureUsesNone {
				continue
			}
			subresource := s.at(mipLevel, arrayLayer)
			if !SkipBarrier(*subresource, firstUsage) {
				transitions.addSubresource(index, mipLevelCount, arrayLayerCount, mipLevel, arrayLayer, *subresource, firstUsage)
			}
			*subresource = end.subresourceUsage(mipLevel, arrayLayer)
		}
	}
	s.collapse()
}

// subresourceUsageTable is the usage of each subresource of a texture of
// arrayLayerCount array layers, usages[mipLevel*arrayLayerCount+arrayLayer].
type subresourceUsageTable struct {
	arrayLayerCount uint32
	usages          []TextureUses
}

// useSubresources records a use of the subresources selector selects in a
// command buffer whose states of the texture are start and end: a
// subresource's first use makes firstUsage its start state and usage its end; a
// later one moves its end to usage, adding the transition to transitions
// unless transitions is nil or SkipBarrier allows omitting it.
//
// Reference: wgpu-core track/texture.rs TextureTracker::set_single
func useSubresources(transitions *textureTransitionList, index TrackerIndex, start, end *textureState,
	selector TextureSelector, firstUsage, usage TextureUses) {
	mipLevelCount, arrayLayerCount := subresourceCounts(selector, start, end)
	if mipLevelCount == 0 {
		if start.usage == TextureUsesNone {
			start.usage = firstUsage
			end.usage = usage
			return
		}
		if transitions != nil && !SkipBarrier(end.usage, usage) {
			transitions.addSelected(index, TextureSelector{}, end.usage, usage)
		}
		end.usage = usage
		return
	}
	start.split(mipLevelCount, arrayLayerCount)
	end.split(mipLevelCount, arrayLayerCount)
	mipStart, mipEnd, layerStart, layerEnd := selector.bounds(mipLevelCount, arrayLayerCount)
	for mipLevel := mipStart; mipLevel < mipEnd; mipLevel++ {
		for arrayLayer := layerStart; arrayLayer < layerEnd; arrayLayer++ {
			useSubresource(transitions, index, start, end, mipLevel, arrayLayer, firstUsage, usage)
		}
	}
	start.collapse()
	end.collapse()
}

// useSubresource is useSubresources for one subresource of split start and
// end states.
func useSubresource(transitions *textureTransitionList, index TrackerIndex, start, end *textureState,
	mipLevel, arrayLayer uint32, firstUsage, usage TextureUses) {
	startUsage := start.at(mipLevel, arrayLayer)
	endUsage := end.at(mipLevel, arrayLayer)
	if *startUsage == TextureUsesNone {
		*startUsage = firstUsage
		*endUsage = usage
		return
	}
	if transitions != nil && !SkipBarrier(*endUsage, usage) {
		mipLevelCount, arrayLayerCount := end.counts()
		transitions.addSubresource(index, mipLevelCount, arrayLayerCount, mipLevel, arrayLayer, *endUsage, usage)
	}
	*endUsage = usage
}

// layersSharingUsage selects the array layers from arrayLayer up to layerEnd
// on mip level mipLevel that share arrayLayer's usage, of a split state of
// mipLevelCount mip levels and arrayLayerCount array layers.
func (s *textureState) layersSharingUsage(mipLevelCount, arrayLayerCount, mipLevel, arrayLayer, layerEnd uint32) TextureSelector {
	usage := s.subresourceUsage(mipLevel, arrayLayer)
	end := arrayLayer + 1
	for end < layerEnd && s.subresourceUsage(mipLevel, end) == usage {
		end++
	}
	return TextureSelector{
		TextureMipLevelCount: mipLevelCount, TextureArrayLayerCount: arrayLayerCount,
		BaseMipLevel: mipLevel, MipLevelCount: 1,
		BaseArrayLayer: arrayLayer, ArrayLayerCount: end - arrayLayer,
	}
}

// subresourceCounts returns the mip level and array layer counts that
// address the subresources of a usage over selector: those of the first of
// states that is split, or else the selector's. Both are 0 when every state
// is whole and the selector selects every subresource, so the usage needs no
// split.
func subresourceCounts(selector TextureSelector, states ...*textureState) (mipLevelCount, arrayLayerCount uint32) {
	for _, state := range states {
		if !state.isWhole() {
			return state.counts()
		}
	}
	if selector.SelectsAll() {
		return 0, 0
	}
	return selector.TextureMipLevelCount, selector.TextureArrayLayerCount
}
