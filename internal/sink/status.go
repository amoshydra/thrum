package sink

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LocalStatus asks the instance on port what it is doing. ok is false when
// nothing answers, or when the answer is not a sinkrill instance.
//
// The port is on the loopback interface, so this is cheap enough to poll.
func LocalStatus(port int) (*Status, bool) {
	client := &http.Client{Timeout: 700 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var st Status
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, false
	}
	// Another service could return valid JSON on this path; require the fields
	// a real instance always reports.
	if st.Station == "" {
		return nil, false
	}
	return &st, true
}
