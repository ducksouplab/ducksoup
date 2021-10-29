package sfu

func Inspect() interface{} {
	mu.Lock()
	defer mu.Unlock()

	output := make(map[string]interface{})
	for _, room := range roomIndex {
		output[room.shortId] = room.inspect()
	}
	if len(output) > 0 {
		return output
	}
	return nil
}
