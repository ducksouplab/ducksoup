package sfu

func Inspect() any {
	return interactionStoreSingleton.inspect()
}

func (is *interactionStore) inspect() any {
	is.Lock()
	defer is.Unlock()

	report := make(map[string]any)
	for _, i := range is.index { // i is an interaction, not an index
		mixerReport := i.mixer.inspect()
		if mixerReport != nil {
			report[i.id] = mixerReport
		}
	}
	if len(report) > 0 {
		return report
	}
	return nil
}

func (m *mixer) inspect() any {
	report := make(map[string]any)
	for _, slice := range m.sliceIndex {
		report[slice.ID()] = slice.inspect()
	}
	if len(report) > 0 {
		return report
	}
	return nil
}

func (ms *mixerSlice) inspect() any {
	// capitalize for JSON export
	return struct {
		From      string
		Kind      string
		IntputKbs uint64
		OutputKbs uint64
		TargetKbs uint64
	}{
		ms.fromPs.userId,
		ms.input.Kind().String(),
		ms.inputBitrate / 1000,
		ms.outputBitrate / 1000,
		ms.targetBitrate / 1000,
	}
}
