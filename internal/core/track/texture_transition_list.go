//go:build !(js && wasm)

package track

// textureTransitionList collects the transitions of textures' subresources,
// merging each into the one before it when that one covers the array layers
// just before it on the same mip level with the same states.
//
// Reference: wgpu-core track/texture.rs barrier (one PendingTransition per
// mip level and layer range)
type textureTransitionList []TexturePendingTransition

// addSubresource records the transition of one subresource from from to to.
func (t *textureTransitionList) addSubresource(index TrackerIndex, mipLevelCount, arrayLayerCount, mipLevel, arrayLayer uint32, from, to TextureUses) {
	if last := len(*t) - 1; last >= 0 {
		previous := &(*t)[last]
		if previous.Index == index && previous.Usage.From == from && previous.Usage.To == to &&
			previous.Selector.BaseMipLevel == mipLevel &&
			previous.Selector.BaseArrayLayer+previous.Selector.ArrayLayerCount == arrayLayer {
			previous.Selector.ArrayLayerCount++
			return
		}
	}
	*t = append(*t, TexturePendingTransition{
		Index:    index,
		Selector: subresourceSelector(mipLevelCount, arrayLayerCount, mipLevel, arrayLayer),
		Usage:    TextureStateTransition{From: from, To: to},
	})
}

// addSelected records the transition of the subresources selector selects
// from from to to.
func (t *textureTransitionList) addSelected(index TrackerIndex, selector TextureSelector, from, to TextureUses) {
	*t = append(*t, TexturePendingTransition{
		Index:    index,
		Selector: selector,
		Usage:    TextureStateTransition{From: from, To: to},
	})
}
