package sfu

func Inspect() interface{} {
	return interactionStoreSingleton.inspect()
}

func (is *interactionStore) inspect() interface{} {
	is.Lock()
	defer is.Unlock()

	report := make(map[string]interface{})
	for _, interaction := range is.index {
		mixerReport := interaction.mixer.inspect()
		if mixerReport != nil {
			report[interaction.id] = mixerReport
		}
	}
	if len(report) > 0 {
		return report
	}
	return nil
}

func (m *mixer) inspect() interface{} {
	report := make(map[string]interface{})
	for _, slice := range m.sliceIndex {
		report[slice.ID()] = slice.inspect()
	}
	if len(report) > 0 {
		return report
	}
	return nil
}

func (s *mixerSlice) inspect() interface{} {
	// capitalize for JSON export
	return struct {
		From      string
		Kind      string
		IntputKbs uint64
		OutputKbs uint64
		TargetKbs uint64
	}{
		s.fromPs.userId,
		s.input.Kind().String(),
		s.inputBitrate / 1000,
		s.outputBitrate / 1000,
		s.optimalBitrate / 1000,
	}
}
