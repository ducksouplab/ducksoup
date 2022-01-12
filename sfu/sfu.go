package sfu

func Inspect() interface{} {
	return roomStoreSingleton.inspect()
}

func (rs *roomStore) inspect() interface{} {
	rs.Lock()
	defer rs.Unlock()

	report := make(map[string]interface{})
	for _, room := range rs.index {
		mixerReport := room.mixer.inspect()
		if mixerReport != nil {
			report[room.qualifiedId] = mixerReport
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
