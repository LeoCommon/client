package sensors

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Temperature int

func (t Temperature) String() string {
	return fmt.Sprintf("%.2f°C", float32(t)/float32(1000.0))
}

type SensorEntry struct {
	// Critical set point is optional
	Crit  *Temperature `json:"crit,omitempty"`
	Label string       `json:"label,omitempty"`
	Temp  Temperature
}

func (t *SensorEntry) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s: %s", t.Label, t.Temp.String()))

	// Add critical temp if set
	if t.Crit != nil {
		sb.WriteString(fmt.Sprintf(" critical: %s", t.Crit.String()))
	}

	return sb.String()
}

// getWhitelistedSensors stores a list of sensors we are interested in
// kXtemp are AMD sensors, coretemp is for Intel and cpu_thermal is for ARM
func getWhitelistedSensors() []string {
	return []string{"coretemp", "k8temp", "k10temp", "cpu_thermal"}
}

func ReadTemperatures() map[string][]SensorEntry {
	var sensors = make(map[string][]SensorEntry)

	// find all hwmon directories
	hwmonPaths, err := filepath.Glob("/sys/class/hwmon/hwmon*")
	if err != nil {
		return sensors
	}

	whitelistedSensors := getWhitelistedSensors()
	for _, hwmonPath := range hwmonPaths {
		nameBytes, err := readStringFromFile(fmt.Sprintf("%s/name", hwmonPath))
		if err != nil {
			continue // unnamed sensor, skip
		}

		// Sensor is not whitelisted
		if !slices.Contains(whitelistedSensors, nameBytes) {
			continue
		}

		// find all temp entries in hwmon directory
		tempPaths, err := filepath.Glob(fmt.Sprintf("%s/temp*_input", hwmonPath))
		if err != nil {
			continue // skip directory if no temp inputs found
		}

		temperatureZones := make([]SensorEntry, 0, len(tempPaths))
		for _, tempPath := range tempPaths {
			// read temperature
			temp := readTemperatureFromFile(tempPath)
			if temp == nil {
				continue
			}

			// Optional: get the critical temperature
			baseName := tempPath[:strings.IndexByte(tempPath, '_')]
			crit := readTemperatureFromFile(fmt.Sprintf("%s_crit", baseName))

			// Optional: read the label
			label, _ := readStringFromFile(fmt.Sprintf("%s_label", baseName))

			// Add the temperatures to the monitor list
			temperatureZones = append(temperatureZones, SensorEntry{
				Temp:  *temp,
				Crit:  crit,
				Label: label,
			})
		}

		// Assign the new list to the sensor name if we found at least one entry
		if len(temperatureZones) > 0 {
			sensors[string(nameBytes)] = temperatureZones
		}
	}

	return sensors
}

func readStringFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func readTemperatureFromFile(path string) *Temperature {
	str, err := readStringFromFile(path)
	if err != nil {
		return nil
	}

	num, err := strconv.Atoi(str)
	if err != nil {
		return nil
	}

	// Create temperature object
	temp := Temperature(num)
	return &temp
}
