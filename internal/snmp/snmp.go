// Package snmp reads UPS-MIB (RFC 1628) over SNMPv3.
package snmp

import (
	"context"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	oidBatteryStatus    = "1.3.6.1.2.1.33.1.2.1.0"
	oidSecondsOnBattery = "1.3.6.1.2.1.33.1.2.2.0"
	oidMinutesRemaining = "1.3.6.1.2.1.33.1.2.3.0"
	oidChargeRemaining  = "1.3.6.1.2.1.33.1.2.4.0"
	oidOutputSource     = "1.3.6.1.2.1.33.1.4.1.0"
	oidInputVoltage     = "1.3.6.1.2.1.33.1.3.3.1.3.1"
	oidOutputLoad       = "1.3.6.1.2.1.33.1.4.4.1.5.1"
)

const (
	StateOnline      = "online"
	StateOnBattery   = "on_battery"
	StateLowBattery  = "low_battery"
	StateUnreachable = "unreachable"
	StateUnknown     = "unknown"
)

// Target is one SNMPv3 UPS.
type Target struct {
	Host          string
	Port          uint16
	User          string
	SecurityLevel string // authNoPriv or authPriv
	AuthProtocol  string
	AuthPassword  string
	PrivProtocol  string
	PrivPassword  string
}

// Reading is one poll of UPS-MIB.
type Reading struct {
	State            string
	MinutesRemaining *int
	SecondsOnBattery *int
	InputVoltage     *int
	LoadPercent      *int
	ChargePercent    *int
	Error            string
}

// Poll queries the UPS. A timeout or auth failure is StateUnreachable.
func Poll(t Target) Reading {
	if t.Port == 0 {
		t.Port = 161
	}
	params := &gosnmp.GoSNMP{
		Target:  t.Host,
		Port:    t.Port,
		Version: gosnmp.Version3,
		Timeout: 2 * time.Second,
		Retries: 1,
		MaxOids: gosnmp.MaxOids,
		// Eaton and MGE cards often answer from a different address or port
		// than the one queried. A connected UDP socket drops those packets
		// and the poll looks like a timeout. NUT uses an unconnected socket.
		UseUnconnectedUDPSocket: true,
		Context:                 context.Background(),
		Logger:                  gosnmp.NewLogger(log.New(io.Discard, "", 0)),
		SecurityModel:           gosnmp.UserSecurityModel,
		MsgFlags:                gosnmp.AuthNoPriv,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 t.User,
			AuthenticationProtocol:   authProtocol(t.AuthProtocol),
			AuthenticationPassphrase: t.AuthPassword,
		},
	}
	if t.SecurityLevel == "authPriv" {
		params.MsgFlags = gosnmp.AuthPriv
		usp := params.SecurityParameters.(*gosnmp.UsmSecurityParameters)
		usp.PrivacyProtocol = privProtocol(t.PrivProtocol)
		usp.PrivacyPassphrase = t.PrivPassword
	}
	if err := params.Connect(); err != nil {
		return Reading{State: StateUnreachable, Error: err.Error()}
	}
	defer params.Conn.Close()

	// Many UPS agents answer one object and ignore a combined GET. NUT does the same.
	oids := []string{
		oidBatteryStatus,
		oidSecondsOnBattery,
		oidMinutesRemaining,
		oidChargeRemaining,
		oidOutputSource,
		oidInputVoltage,
		oidOutputLoad,
	}
	vals := map[string]int{}
	var firstErr string
	got := 0
	for _, oid := range oids {
		pkt, err := params.Get([]string{oid})
		if err != nil {
			if firstErr == "" {
				firstErr = err.Error()
			}
			if strings.Contains(err.Error(), "timeout") {
				break
			}
			continue
		}
		if pkt.Error != gosnmp.NoError {
			if firstErr == "" {
				firstErr = pkt.Error.String()
			}
			continue
		}
		got++
		for _, v := range pkt.Variables {
			n, ok := pduInt(v)
			if !ok {
				continue
			}
			vals[v.Name] = n
		}
	}
	if got == 0 {
		return Reading{State: StateUnreachable, Error: firstErr}
	}
	out := Interpret(vals[normalize(oidOutputSource)], vals[normalize(oidBatteryStatus)])
	r := Reading{State: out}
	r.SecondsOnBattery = optional(vals, oidSecondsOnBattery)
	r.MinutesRemaining = optional(vals, oidMinutesRemaining)
	r.ChargePercent = optional(vals, oidChargeRemaining)
	r.InputVoltage = optional(vals, oidInputVoltage)
	r.LoadPercent = optional(vals, oidOutputLoad)
	if _, ok := vals[normalize(oidOutputSource)]; !ok {
		r.State = StateUnknown
		r.Error = "upsOutputSource missing"
	}
	return r
}

func optional(vals map[string]int, oid string) *int {
	n, ok := vals[normalize(oid)]
	if !ok {
		return nil
	}
	return &n
}

func normalize(oid string) string {
	if len(oid) > 0 && oid[0] != '.' {
		return "." + oid
	}
	return oid
}

func pduInt(v gosnmp.SnmpPDU) (int, bool) {
	switch n := v.Value.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	default:
		return 0, false
	}
}

// Interpret maps UPS-MIB upsOutputSource and upsBatteryStatus to a state name.
// outputSource and batteryStatus use the RFC 1628 enumerations.
func Interpret(outputSource, batteryStatus int) string {
	low := batteryStatus == 3 || batteryStatus == 4
	switch outputSource {
	case 3, 4, 6, 7: // normal, bypass, booster, reducer
		return StateOnline
	case 5: // battery
		if low {
			return StateLowBattery
		}
		return StateOnBattery
	case 2: // none
		return StateLowBattery
	default:
		return StateUnknown
	}
}

func authProtocol(name string) gosnmp.SnmpV3AuthProtocol {
	switch name {
	case "MD5":
		return gosnmp.MD5
	case "SHA224":
		return gosnmp.SHA224
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.SHA
	}
}

func privProtocol(name string) gosnmp.SnmpV3PrivProtocol {
	switch name {
	case "DES":
		return gosnmp.DES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	default:
		return gosnmp.AES
	}
}

// Protocols lists the auth and privacy names the UI may send.
func Protocols() (auth []string, priv []string) {
	return []string{"MD5", "SHA", "SHA224", "SHA256", "SHA384", "SHA512"},
		[]string{"DES", "AES", "AES192", "AES256"}
}

// ValidateTarget checks the fields required before a poll is attempted.
func ValidateTarget(t Target) error {
	if t.Host == "" || t.User == "" || t.AuthPassword == "" {
		return fmt.Errorf("host, user, and auth password are required")
	}
	if t.SecurityLevel != "authNoPriv" && t.SecurityLevel != "authPriv" {
		return fmt.Errorf("security level must be authNoPriv or authPriv")
	}
	if t.SecurityLevel == "authPriv" && t.PrivPassword == "" {
		return fmt.Errorf("privacy password is required for authPriv")
	}
	auth, priv := Protocols()
	if !contains(auth, t.AuthProtocol) {
		return fmt.Errorf("unknown auth protocol %q", t.AuthProtocol)
	}
	if t.SecurityLevel == "authPriv" && !contains(priv, t.PrivProtocol) {
		return fmt.Errorf("unknown privacy protocol %q", t.PrivProtocol)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
