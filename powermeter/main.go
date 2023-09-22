package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/godbus/dbus/introspect"
	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"
)

/* Configuration */
var (
	broker     = "192.168.2.8"
	brokerPort = 1883
	topic1      = "dev37/r/em_cur_fast"
	topic2      = "dev37/r/em_tot_solar"
	topic3      = "dev37/r/em_tot_grid"
	clientId   = "grid-bridge"
	username   = "ha"
	password   = "ah"
)

var P1 float64 = 0.00
var P2 float64 = 0.00
var P3 float64 = 0.00
var psum float64 = 0.00
var psum_update bool = true
var value_correction bool = false
var conn, err = dbus.SystemBus()

type singlePhase struct {
	voltage float32 // Volts: 230,0
	curent  float32 // Amps: 8,3
	power   float32 // Watts: 1909
	forward float64 // kWh, purchased power
	reverse float64 // kWh, sold power
}

const intro = `
<node>
   <interface name="com.victronenergy.BusItem">
    <signal name="PropertiesChanged">
      <arg type="a{sv}" name="properties" />
    </signal>
    <method name="SetValue">
      <arg direction="in"  type="v" name="value" />
      <arg direction="out" type="i" />
    </method>
    <method name="GetText">
      <arg direction="out" type="s" />
    </method>
    <method name="GetValue">
      <arg direction="out" type="v" />
    </method>
    </interface>` + introspect.IntrospectDataString + `</node> `

type objectpath string

var victronValues = map[int]map[objectpath]dbus.Variant{
	// 0: This will be used to store the VALUE variant
	0: map[objectpath]dbus.Variant{},
	// 1: This will be used to store the STRING variant
	1: map[objectpath]dbus.Variant{},
}

func (f objectpath) GetValue() (dbus.Variant, *dbus.Error) {
	log.Debug("GetValue() called for ", f)
	log.Debug("...returning ", victronValues[0][f])
	return victronValues[0][f], nil
}
func (f objectpath) GetText() (string, *dbus.Error) {
	log.Debug("GetText() called for ", f)
	log.Debug("...returning ", victronValues[1][f])
	// Why does this end up ""SOMEVAL"" ... trim it I guess
	return strings.Trim(victronValues[1][f].String(), "\""), nil
}

func init() {
	lvl, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		lvl = "info"
	}

	ll, err := log.ParseLevel(lvl)
	if err != nil {
		ll = log.DebugLevel
	}

	log.SetLevel(ll)
}

func main() {
	// Parse command line arguments
	flag.StringVar(&broker, "broker", broker, "MQTT broker address")
	flag.IntVar(&brokerPort, "port", brokerPort, "MQTT broker port")
	flag.StringVar(&topic1, "topic", topic1, "MQTT topic prefix")
	flag.StringVar(&clientId, "client-id", clientId, "MQTT client id")
	flag.StringVar(&username, "username", username, "MQTT username")
	flag.StringVar(&password, "password", password, "MQTT password")
	flag.Parse()

	// Need to implement following paths:
	// https://github.com/victronenergy/venus/wiki/dbus#grid-meter
	// also in system.py
	victronValues[0]["/Connected"] = dbus.MakeVariant(1)
	victronValues[1]["/Connected"] = dbus.MakeVariant("1")

	victronValues[0]["/CustomName"] = dbus.MakeVariant("Grid meter")
	victronValues[1]["/CustomName"] = dbus.MakeVariant("Grid meter")

	victronValues[0]["/DeviceInstance"] = dbus.MakeVariant(30)
	victronValues[1]["/DeviceInstance"] = dbus.MakeVariant("30")

	// also in system.py
	victronValues[0]["/DeviceType"] = dbus.MakeVariant(71)
	victronValues[1]["/DeviceType"] = dbus.MakeVariant("71")

	victronValues[0]["/ErrorCode"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	victronValues[1]["/ErrorCode"] = dbus.MakeVariant("0")

	victronValues[0]["/FirmwareVersion"] = dbus.MakeVariant(2)
	victronValues[1]["/FirmwareVersion"] = dbus.MakeVariant("2")

	// also in system.py
	victronValues[0]["/Mgmt/Connection"] = dbus.MakeVariant("/dev/ttyUSB0")
	victronValues[1]["/Mgmt/Connection"] = dbus.MakeVariant("/dev/ttyUSB0")

	victronValues[0]["/Mgmt/ProcessName"] = dbus.MakeVariant("/opt/color-control/dbus-cgwacs/dbus-cgwacs")
	victronValues[1]["/Mgmt/ProcessName"] = dbus.MakeVariant("/opt/color-control/dbus-cgwacs/dbus-cgwacs")

	victronValues[0]["/Mgmt/ProcessVersion"] = dbus.MakeVariant("1.8.0")
	victronValues[1]["/Mgmt/ProcessVersion"] = dbus.MakeVariant("1.8.0")

	victronValues[0]["/Position"] = dbus.MakeVariantWithSignature(0, dbus.SignatureOf(123))
	victronValues[1]["/Position"] = dbus.MakeVariant("0")

	// also in system.py
	victronValues[0]["/ProductId"] = dbus.MakeVariant(45058)
	victronValues[1]["/ProductId"] = dbus.MakeVariant("45058")

	// also in system.py
	victronValues[0]["/ProductName"] = dbus.MakeVariant("Grid meter")
	victronValues[1]["/ProductName"] = dbus.MakeVariant("Grid meter")

	victronValues[0]["/Serial"] = dbus.MakeVariant("BP98305081235")
	victronValues[1]["/Serial"] = dbus.MakeVariant("BP98305081235")

	// Provide some initial values... note that the values must be a valid formt otherwise dbus_systemcalc.py exits like this:
	// @400000005ecc11bf3782b374   File "/opt/victronenergy/dbus-systemcalc-py/dbus_systemcalc.py", line 386, in _handletimertick
	// @400000005ecc11bf37aa251c     self._updatevalues()
	// @400000005ecc11bf380e74cc   File "/opt/victronenergy/dbus-systemcalc-py/dbus_systemcalc.py", line 678, in _updatevalues
	// @400000005ecc11bf383ab4ec     c = _safeadd(c, p, pvpower)
	// @400000005ecc11bf386c9674   File "/opt/victronenergy/dbus-systemcalc-py/sc_utils.py", line 13, in safeadd
	// @400000005ecc11bf387b28ec     return sum(values) if values else None
	// @400000005ecc11bf38b2bb7c TypeError: unsupported operand type(s) for +: 'int' and 'unicode'
	//
	victronValues[0]["/Ac/L1/Power"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Power"] = dbus.MakeVariant("0 W")
	victronValues[0]["/Ac/L2/Power"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Power"] = dbus.MakeVariant("0 W")
	victronValues[0]["/Ac/L3/Power"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Power"] = dbus.MakeVariant("0 W")

	victronValues[0]["/Ac/L1/Voltage"] = dbus.MakeVariant(230)
	victronValues[1]["/Ac/L1/Voltage"] = dbus.MakeVariant("230 V")
	victronValues[0]["/Ac/L2/Voltage"] = dbus.MakeVariant(230)
	victronValues[1]["/Ac/L2/Voltage"] = dbus.MakeVariant("230 V")
	victronValues[0]["/Ac/L3/Voltage"] = dbus.MakeVariant(230)
	victronValues[1]["/Ac/L3/Voltage"] = dbus.MakeVariant("230 V")

	victronValues[0]["/Ac/L1/Current"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Current"] = dbus.MakeVariant("0 A")
	victronValues[0]["/Ac/L2/Current"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Current"] = dbus.MakeVariant("0 A")
	victronValues[0]["/Ac/L3/Current"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Current"] = dbus.MakeVariant("0 A")

	victronValues[0]["/Ac/L1/Energy/Forward"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L2/Energy/Forward"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Energy/Forward"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L3/Energy/Forward"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Energy/Forward"] = dbus.MakeVariant("0 kWh")

	victronValues[0]["/Ac/L1/Energy/Reverse"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L1/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L2/Energy/Reverse"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L2/Energy/Reverse"] = dbus.MakeVariant("0 kWh")
	victronValues[0]["/Ac/L3/Energy/Reverse"] = dbus.MakeVariant(0.0)
	victronValues[1]["/Ac/L3/Energy/Reverse"] = dbus.MakeVariant("0 kWh")

	basicPaths := []dbus.ObjectPath{
		"/Connected",
		"/CustomName",
		"/DeviceInstance",
		"/DeviceType",
		"/ErrorCode",
		"/FirmwareVersion",
		"/Mgmt/Connection",
		"/Mgmt/ProcessName",
		"/Mgmt/ProcessVersion",
		"/Position",
		"/ProductId",
		"/ProductName",
		"/Serial",
	}

	updatingPaths := []dbus.ObjectPath{
		"/Ac/L1/Power",
		"/Ac/L2/Power",
		"/Ac/L3/Power",
		"/Ac/L1/Voltage",
		"/Ac/L2/Voltage",
		"/Ac/L3/Voltage",
		"/Ac/L1/Current",
		"/Ac/L2/Current",
		"/Ac/L3/Current",
		"/Ac/L1/Energy/Forward",
		"/Ac/L2/Energy/Forward",
		"/Ac/L3/Energy/Forward",
		"/Ac/L1/Energy/Reverse",
		"/Ac/L2/Energy/Reverse",
		"/Ac/L3/Energy/Reverse",
	}

	defer conn.Close()

	// Some of the victron stuff requires it be called grid.cgwacs... using the only known valid value (from the simulator)
	// This can _probably_ be changed as long as it matches com.victronenergy.grid.cgwacs_*
	reply, err := conn.RequestName("com.victronenergy.grid.cgwacs_ttyUSB0_di30_mb1",
		dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Panic("Something went horribly wrong in the dbus connection")
		panic(err)
	}

	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Panic("name cgwacs_ttyUSB0_di30_mb1 already taken on dbus.")
		os.Exit(1)
	}

	for i, s := range basicPaths {
		log.Debug("Registering dbus basic path #", i, ": ", s)
		conn.Export(objectpath(s), s, "com.victronenergy.BusItem")
		conn.Export(introspect.Introspectable(intro), s, "org.freedesktop.DBus.Introspectable")
	}

	for i, s := range updatingPaths {
		log.Debug("Registering dbus update path #", i, ": ", s)
		conn.Export(objectpath(s), s, "com.victronenergy.BusItem")
		conn.Export(introspect.Introspectable(intro), s, "org.freedesktop.DBus.Introspectable")
	}

	log.Info("Successfully connected to dbus and registered as a pvinverter... Commencing reading grid")

	// MQTT Subscripte
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", broker, brokerPort))
	opts.SetClientID(clientId)
	opts.SetUsername(username)
	opts.SetPassword(password)
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	sub(client, topic1)
	sub(client, topic2)
	sub(client, topic3)
	// Infinite loop
	for true {
		// fmt.Println("Infinite Loop entered")
		time.Sleep(time.Second)
	}

	// This is a forever loop^^
	panic("Error: We terminated.... how did we ever get here?")
}

/* MQTT Subscribe Function */
func sub(client mqtt.Client, topic string) {
	token := client.Subscribe(topic, 1, nil)
	token.Wait()
	log.Info("Subscribed to topic: " + topic)
}

/* MQTT Publish Function */
func publish(client mqtt.Client) {
	num := 10
	for i := 0; i < num; i++ {
		text := fmt.Sprintf("Message %d", i)
		token := client.Publish("topic/test", 0, false, text)
		token.Wait()
		time.Sleep(time.Second)
	}
}

/* Write dbus Values to Victron handler */
func updateVariant(value float64, unit string, path string) {
	emit := make(map[string]dbus.Variant)
	emit["Text"] = dbus.MakeVariant(fmt.Sprintf("%.2f", value) + unit)
	emit["Value"] = dbus.MakeVariant(float64(value))
	victronValues[0][objectpath(path)] = emit["Value"]
	victronValues[1][objectpath(path)] = emit["Text"]
	conn.Emit(dbus.ObjectPath(path), "com.victronenergy.BusItem.PropertiesChanged", emit)
}

/* Convert binary to float64 */
func bin2Float64(bin string) float64 {
	foostring := string(bin)
	foostring = strings.ReplaceAll(foostring, " ", "")
	result, err := strconv.ParseFloat(foostring, 64)
	if err != nil {
		panic(err)
	}
	return result
}

/* Called if connection is established */
var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Info(fmt.Sprintf("Connected to broker %s:%d", broker, brokerPort))
}

/* Called if connection is lost  */
var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Info(fmt.Sprintf("Connect lost: %v", err))
	os.Exit(1)
}

/* Search for string with regex */
func ContainString(searchstring string, str string) bool {
	var obj bool

	obj, err = regexp.MatchString(searchstring, str)

	if err != nil {
		panic(err)
	}

	return obj
}

/* MQTT Subscribe Handler */
var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {

	log.Debug(fmt.Sprintf("Received message: %s from topic: %s\n", msg.Payload(), msg.Topic()))
	value_correction = false

	// Power
	if ContainString(".*dev37/r/em_cur_fast$", msg.Topic()) {
		// P1 = bin2Float64(string(msg.Payload()))
		P1, err := strconv.ParseFloat(string(msg.Payload()), 64)
		if err != nil {
		      fmt.Println("Error:", err)
		      return
		}
		updateVariant(float64(P1), "W", "/Ac/Power")
		updateVariant(float64(P1), "W", "/Ac/L1/Power")
	}

	// /Ac/Energy/Reverse
	if ContainString(".*dev37/r/em_tot_solar$", msg.Topic()) {
		EL3 := bin2Float64(string(msg.Payload()))
		updateVariant(float64(EL3), "kWh", "/Ac/Energy/Reverse")
		log.Debug(fmt.Sprintf("Energy: %.3f kWh", EL3))
	}
	// /Ac/Energy/Forward
	if ContainString(".*dev37/r/em_tot_grid$", msg.Topic()) {
		EL3 := bin2Float64(string(msg.Payload()))
		updateVariant(float64(EL3), "kWh", "/Ac/Energy/Forward")
		log.Debug(fmt.Sprintf("Energy: %.3f kWh", EL3))
	}
}
