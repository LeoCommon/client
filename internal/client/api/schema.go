package api

import (
	"encoding/json"
	"time"
)

// Some structs to handle the Json, coming from the server
type FixedJob struct {
	StartTime time.Time
	EndTime   time.Time
	Arguments map[string]string `json:"arguments"`
	States    map[string]string `json:"states"`
	Id        string            `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Status    string            `json:"status"`
	Sensors   []string          `json:"sensors"`
}

// Custom unmarshaller so we can use time.Time within the go code and avoid time mistakes
func (j *FixedJob) UnmarshalJSON(data []byte) error {
	type Alias FixedJob // Create an alias for the struct to avoid infinite recursion
	aux := &struct {
		*Alias
		StartTime int64 `json:"start_time"`
		EndTime   int64 `json:"end_time"`
	}{
		Alias: (*Alias)(j),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	j.StartTime = time.Unix(aux.StartTime, 0).UTC()
	j.EndTime = time.Unix(aux.EndTime, 0).UTC()
	return nil
}

func (j *FixedJob) Json() string {
	js, _ := json.Marshal(j)
	return string(js)
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
