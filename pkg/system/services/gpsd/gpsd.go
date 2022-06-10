package gpsd

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const ()

type GPSDFixSignal struct {
	Time              float64
	Mode              int32
	TimeUncertainty   float64
	Lat               float64
	Lon               float64
	HorizUncertainty  float64
	AltMSL            float64
	AltUncertainty    float64
	Course            float64
	CourseUncertainty float64
	Speed             float64
	SpeedUncertainty  float64
	Climb             float64
	ClimbUncertainty  float64
	DeviceName        string
}

func (s *gpsdService) initialize() {
	if err := s.conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/gpsd"),
		dbus.WithMatchInterface("org.gpsd"),
	); err != nil {
		panic(err)
	}

	c := make(chan *dbus.Signal, 10)
	s.conn.Signal(c)

	// #fixme #breaking This blocks the entire process, read these from another thread
	for v := range c {
		if v.Name == "org.gpsd.fix" {
			fix := GPSDFixSignal{}
			fix.Time = v.Body[0].(float64)
			fix.Mode = v.Body[1].(int32)
			fix.TimeUncertainty = v.Body[2].(float64)
			fix.Lat = v.Body[3].(float64)
			fix.Lon = v.Body[4].(float64)
			fix.HorizUncertainty = v.Body[5].(float64)
			fix.AltMSL = v.Body[6].(float64)
			fix.AltUncertainty = v.Body[7].(float64)
			fix.Course = v.Body[8].(float64)
			fix.CourseUncertainty = v.Body[9].(float64)
			fix.Speed = v.Body[10].(float64)
			fix.SpeedUncertainty = v.Body[11].(float64)
			fix.Climb = v.Body[12].(float64)
			fix.ClimbUncertainty = v.Body[13].(float64)
			fix.DeviceName = v.Body[14].(string)

			fmt.Println(fix)
		}
	}
}

type gpsdService struct {
	conn *dbus.Conn
}

func NewService(conn *dbus.Conn) gpsdService {
	e := gpsdService{conn: conn}
	e.initialize()

	return e
}
