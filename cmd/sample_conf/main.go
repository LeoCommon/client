package main

import (
	"bytes"
	"os"
	"reflect"
	"time"

	"github.com/LeoCommon/client/internal/client/config"
	"github.com/LeoCommon/client/pkg/log"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

// Sample config export function
func setDefaultFunc(v reflect.Value) {
	switch v.Kind() {

	case reflect.Slice:
		elemType := v.Type().Elem()

		// Find the SupportedOptions method of the type
		allMethod := reflect.New(elemType).MethodByName("SupportedOptions")
		if allMethod.IsValid() {
			v.Set(allMethod.Call(nil)[0])
		} else {
			// Handle other slice types
			if v.IsNil() {
				newElem := reflect.New(elemType)
				v.Set(reflect.Append(v, newElem.Elem()))
			}
			setDefaultFunc(v.Index(0))
		}
	case reflect.Ptr:
		// Create new instance for pointer
		if v.IsNil() {
			elemType := v.Type().Elem()
			newElem := reflect.New(elemType)
			v.Set(newElem)
		}
		setDefaultFunc(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := v.Type().Field(i)

			// Make durations not way too small
			if fieldType.Type.ConvertibleTo(reflect.TypeOf(time.Duration(0))) {
				field.SetInt(int64(time.Duration(10 * time.Second)))
				continue
			}

			switch field.Kind() {
			case reflect.Slice:
				setDefaultFunc(field)

			case reflect.Ptr:
				// Create new instance for pointer
				if field.IsNil() {
					elemType := field.Type().Elem()
					newElem := reflect.New(elemType)
					field.Set(newElem)
				}
				setDefaultFunc(field.Elem())
			case reflect.Struct:
				setDefaultFunc(field)
			case reflect.String:
				if field.String() == "" {
					field.SetString(v.Type().Field(i).Name)
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				if field.Int() == 0 {
					field.SetInt(42)
				}

			case reflect.Float32, reflect.Float64:
				if field.Float() == 0.0 {
					field.SetFloat(3.14)
				}
			case reflect.Bool:
				if !field.Bool() {
					field.SetBool(true)
				}
			}

		}
	}
}

func setDefaultsRecursive(v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)

		if field.Kind() == reflect.Ptr && field.Elem().Kind() == reflect.Struct {
			setDefaultsRecursive(field.Elem())
		} else {
			setDefaultFunc(field)
		}
	}
}

func main() {
	// Create the new config manager and load the configuration
	// Create a copy of the config with all fields set to their default values
	var cf = config.MainConfig{}
	cfPtr := &cf

	// Modify the config tree in a way that gives us "default" values that bypass "omitempty"
	setDefaultsRecursive(reflect.ValueOf(cfPtr).Elem())

	defaultConfigBytes, err := toml.Marshal(cfPtr)
	if err != nil {
		panic(err)
	}

	// Marshal the default config to TOML
	var buf bytes.Buffer
	if _, err := buf.Write(defaultConfigBytes); err != nil {
		panic(err)
	}

	if err := os.WriteFile("./config/config.toml", defaultConfigBytes, 0644); err != nil {
		log.Error("Failed to write config file", zap.Error(err))
		panic(err)
	}

}
