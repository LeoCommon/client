package api

// Some structs to handle the Json, coming from the server
type FixedJob struct {
	Arguments map[string]string `json:"arguments"`
	States    map[string]string `json:"states"`
	Id        string            `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Status    string            `json:"status"`
	Sensors   []string          `json:"sensors"`
	StartTime int64             `json:"start_time"`
	EndTime   int64             `json:"end_time"`
}

type FixedJobResponse struct {
	Message string
	Data    []FixedJob
	Code    int
}

type SensorStatus struct {
	OsVersion          string  `json:"os_version"`
	LTE                string  `json:"LTE"`
	WiFi               string  `json:"WiFi"`
	Ethernet           string  `json:"Ethernet"`
	StatusTime         int64   `json:"status_time"`
	LocationLat        float64 `json:"location_lat"`
	LocationLon        float64 `json:"location_lon"`
	TemperatureCelsius float64 `json:"temperature_celsius"`
}
