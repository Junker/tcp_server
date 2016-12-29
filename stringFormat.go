package tcp_server

func stringFormatWithBS(str string) string {
	if str == "" {
		return ""
	}
	ts := ""
	for _, chr := range str {
		if chr == 8 {
			if ts != "" && len(ts) > 1 {
				ts = ts[:len(ts)-1]
				continue
			} else if len(ts) == 1 {
				ts = ""
				continue
			}
			continue
		}
		if chr < 32 || chr > 126 {
			continue
		}
		ts += string(chr)
	}
	return ts
}
