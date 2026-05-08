// Copyright 2025 The HuaTuo Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package events

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"huatuo-bamai/internal/bpf"
	"huatuo-bamai/internal/storage"
	"huatuo-bamai/pkg/metric"
	"huatuo-bamai/pkg/tracing"
)

//go:generate $BPF_COMPILE $BPF_INCLUDE -s $BPF_DIR/ras.c -o $BPF_DIR/ras.o
type rasTracing struct {
	count uint64
}

const (
	HW_ERR_MCE          = 0
	HW_ERR_EDAC         = 1
	HW_ERR_NON_STANDARD = 2
	HW_ERR_AER_EVENT    = 3
)

var (
	ErrTypeCorrected   = "CORRECTED"
	ErrTypeUncorrected = "UNCORRECTED"
	ErrTypeRecovPanic  = "RECOVERABLE/PANIC"
	ErrTypeFatal       = "FATAL"
)

// The dynamic_array info is just at the very last place of the event
// struct. We don't know the exact length of the info because it depends
// on the driver. Just read the whole 512 bytes of the perf event output
// Info.
//
// The length of the other part besids data[] are:
// struct trace_event_raw_mc_event: 64 - 4 = 60
// struct trace_event_raw_non_standard_event: 60 - 4 = 56
// struct trace_event_raw_aer_event: 40 - 4 = 36
const (
	RAS_PERFEVENT_INFO_SIZE       = 512
	DETAIL_INFO_SIZE_EDAC         = RAS_PERFEVENT_INFO_SIZE - 60
	DETAIL_INFO_SIZE_NON_STANDARD = RAS_PERFEVENT_INFO_SIZE - 56
	DETAIL_INFO_SIZE_AER          = RAS_PERFEVENT_INFO_SIZE - 36
)

type rasPerfEvent struct {
	Type      uint32
	Corrected uint32
	Timestamp uint64
	Info      [RAS_PERFEVENT_INFO_SIZE]byte
}

type RasTracingData struct {
	Device    string `json:"dev"`
	Event     string `json:"event"`
	ErrType   string `json:"type"`
	Timestamp uint64 `json:"timestamp"`
	Info      string `json:"info"`
}

var (
	interruptsPath        = "/proc/interrupts"
	thresholdCount uint64 = 0
)

func init() {
	tracing.RegisterEventTracing("ras", newRasTracing)
}

func newRasTracing() (*tracing.EventTracingAttr, error) {
	return &tracing.EventTracingAttr{
		TracingData: &rasTracing{},
		Interval:    60,
		Flag:        tracing.FlagTracing | tracing.FlagMetric,
	}, nil
}

func CopyFromOffset(src []byte, offset, length int) ([]byte, error) {
	if offset < 0 || offset >= len(src) {
		return nil, fmt.Errorf("offset out of bounds")
	}
	if length <= 0 || offset+length > len(src) {
		return nil, fmt.Errorf("invalid length")
	}

	dst := make([]byte, length)
	if n := copy(dst, src[offset:offset+length]); n != length {
		return nil, fmt.Errorf("incomplete copy")
	}
	return dst, nil
}

const (
	// Correctable errors status
	PciErrCorRcvr     uint32 = 0x00000001 /* Receiver Error Status */
	PciErrCorBadTlp   uint32 = 0x00000040 /* Bad TLP Status */
	PciErrCorBadDllp  uint32 = 0x00000080 /* Bad DLLP Status */
	PciErrCorRepRoll  uint32 = 0x00000100 /* REPLAY_NUM Rollover */
	PciErrCorRepTimer uint32 = 0x00001000 /* Replay Timer Timeout */
	PciErrCorAdvNfat  uint32 = 0x00002000 /* Advisory Non-Fatal */
	PciErrCorInternal uint32 = 0x00004000 /* Corrected Internal */
	PciErrCorLogOver  uint32 = 0x00008000 /* Header Log Overflow */

	// Uncorrectable errors status
	PciErrUncUnd       uint32 = 0x00000001 /* Undefined */
	PciErrUncDlp       uint32 = 0x00000010 /* Data Link Protocol */
	PciErrUncSurpdn    uint32 = 0x00000020 /* Surprise Down */
	PciErrUncPoisonTlp uint32 = 0x00001000 /* Poisoned TLP */
	PciErrUncFcp       uint32 = 0x00002000 /* Flow Control Protocol */
	PciErrUncCompTime  uint32 = 0x00004000 /* Completion Timeout */
	PciErrUncCompAbort uint32 = 0x00008000 /* Completer Abort */
	PciErrUncUnxComp   uint32 = 0x00010000 /* Unexpected Completion */
	PciErrUncRxOver    uint32 = 0x00020000 /* Receiver Overflow */
	PciErrUncMalfTlp   uint32 = 0x00040000 /* Malformed TLP */
	PciErrUncEcrc      uint32 = 0x00080000 /* ECRC Error Status */
	PciErrUncUnsup     uint32 = 0x00100000 /* Unsupported Request */
	PciErrUncAscv      uint32 = 0x00200000 /* ACS Violation */
	PciErrUncIntn      uint32 = 0x00400000 /* internal error */
	PciErrUncMcptlp    uint32 = 0x00800000 /* MC blocked TLP */
	PciErrUncAtomeg    uint32 = 0x01000000 /* Atomic egress blocked */
	PciErrUncTlpPre    uint32 = 0x02000000 /* TLP prefix blocked */
)

var aerCorrectablErrors = map[uint32]string{
	PciErrCorRcvr:     "Receiver Error",
	PciErrCorBadTlp:   "Bad TLP",
	PciErrCorBadDllp:  "PciErrCorBadDllp",
	PciErrCorRepRoll:  "RELAY_NUM Rollover",
	PciErrCorRepTimer: "Replay Timer Timeout",
	PciErrCorAdvNfat:  "Advisory Non-Fatal Error",
	PciErrCorInternal: "Corrected Internal Error",
	PciErrCorLogOver:  "Header Log Overflow",
}

var aerUncorrectableErrors = map[uint32]string{
	PciErrUncUnd:       "Undefined",
	PciErrUncDlp:       "Data Link Protocol Error",
	PciErrUncSurpdn:    "Surprise Down Error",
	PciErrUncPoisonTlp: "Poisoned TLP",
	PciErrUncFcp:       "Flow Control Protocol Error",
	PciErrUncCompTime:  "Completion Timeout",
	PciErrUncCompAbort: "Completer Abort",
	PciErrUncUnxComp:   "Unexpected Completion",
	PciErrUncRxOver:    "Receiver Overflow",
	PciErrUncMalfTlp:   "Malformed TLP",
	PciErrUncEcrc:      "ECRC Error",
	PciErrUncUnsup:     "Unsupported Request Error",
	PciErrUncAscv:      "ACS Violation",
	PciErrUncIntn:      "Uncorrectable Internal Error",
	PciErrUncMcptlp:    "MC Blocked TLP",
	PciErrUncAtomeg:    "AtomicOp Egress Blocked",
	PciErrUncTlpPre:    "TLP Prefix Blocked Error",
}

func pciErr(key uint32, isCorrectable bool) string {
	if isCorrectable {
		if val, exists := aerCorrectablErrors[key]; exists {
			return val
		}
	} else {
		if val, exists := aerUncorrectableErrors[key]; exists {
			return val
		}
	}

	return "not supported"
}

func ErrType(corrected uint32) string {
	if corrected != 0 {
		return ErrTypeCorrected
	}

	return ErrTypeUncorrected
}

func buildRasMceTracerData(data *rasPerfEvent) (*RasTracingData, error) {
	// trace payload data
	type tracepointMcePayload struct {
		Pad       uint64
		Mcgcap    uint64
		McgStatus uint64
		Status    uint64
		Addr      uint64
		Misc      uint64
		Synd      uint64
		Ipid      uint64
		Ip        uint64
		Tsc       uint64
		Walltime  uint64
		Cpu       uint32
		Cpuid     uint32
		Apicid    uint32
		Socketid  uint32
		Cs        uint8
		Bank      uint8
		Cpuvendor uint8
	}

	payload := &tracepointMcePayload{}

	reader := bytes.NewReader(data.Info[:])
	err := binary.Read(reader, binary.LittleEndian, payload)
	if err != nil {
		return nil, fmt.Errorf("parse mec payload: %w", err)
	}

	return &RasTracingData{
		Timestamp: data.Timestamp,
		Device:    "CPU/MEM",
		Event:     "MCE",
		ErrType:   ErrType(data.Corrected),
		Info: fmt.Sprintf("CPU: %d, MCGc/s: %x/%x, MC%d: %016x, "+
			"IPID: %016x, ADDR/MISC/SYND: %016x/%016x/%016x, "+
			"RIP: %02x:<%016x>, TSC: %x, PROCESSOR: %x:%x, "+
			"TIME: %d, SOCKET: %x, APIC: %x",
			payload.Cpu, payload.Mcgcap, payload.McgStatus,
			payload.Bank, payload.Status,
			payload.Ipid, payload.Addr, payload.Misc,
			payload.Synd, payload.Cs, payload.Ip,
			payload.Tsc, payload.Cpuvendor,
			payload.Cpuid, payload.Walltime,
			payload.Socketid, payload.Apicid),
	}, nil
}

func buildRasEdacTracerData(data *rasPerfEvent) (*RasTracingData, error) {
	type tracepointEdacPayload struct {
		Pad            uint64
		ErrType        uint32
		ErrorMsgOffset uint32
		LabelOffset    uint32
		ErrCount       uint16
		McIndex        uint8
		TopLayer       int8
		MidLayer       int8
		LowLayer       int8
		ReserveA       [6]uint8
		Addr           uint64
		GrainBits      uint8
		ReserveB       [7]uint8
		Syndrome       uint64
		DriverDetail   uint32
		MsgDetail      [DETAIL_INFO_SIZE_EDAC]byte
	}

	payload := &tracepointEdacPayload{}

	reader := bytes.NewReader(data.Info[:])
	err := binary.Read(reader, binary.LittleEndian, payload)
	if err != nil {
		return nil, fmt.Errorf("parse edac: %w", err)
	}

	msgRaw := payload.MsgDetail[:]

	// Error message
	msgOff := payload.ErrorMsgOffset&0xffff - 60
	msgEnd := bytes.IndexByte(msgRaw[msgOff:], 0)
	msg := string(msgRaw[msgOff : int(msgOff)+msgEnd])

	// Label
	labelOff := payload.LabelOffset&0xffff - 60
	labelEnd := bytes.IndexByte(msgRaw[labelOff:], 0)
	label := string(msgRaw[labelOff : int(labelOff)+labelEnd])

	// Driver info
	driverOff := payload.DriverDetail&0xffff - 60
	driverEnd := bytes.IndexByte(msgRaw[driverOff:], 0)
	driver := string(msgRaw[driverOff : int(driverOff)+driverEnd])

	return &RasTracingData{
		Timestamp: data.Timestamp,
		Device:    "MEM",
		Event:     "EDAC",
		ErrType:   ErrType(data.Corrected),
		Info: fmt.Sprintf("%d %s err: %s on %s "+
			"(mc: %d location:%d:%d:%d "+
			"address: %#x grain:%d syndrome:%#x %s)",
			payload.ErrCount,
			ErrType(data.Corrected),
			msg,
			label,
			payload.McIndex,
			payload.TopLayer,
			payload.MidLayer,
			payload.LowLayer,
			payload.Addr,
			1<<payload.GrainBits,
			payload.Syndrome,
			driver),
	}, nil
}

func buildRasAcpiTracerData(data *rasPerfEvent) (*RasTracingData, error) {
	type tracepointAcpiNonStandardPayload struct {
		Pad          uint64
		SecType      [16]uint8
		FruID        [16]uint8
		FruTxtOffset uint32
		Sev          uint8
		Pattern      [3]uint8
		Len          uint32
		BufOffset    uint32
		Msg          [DETAIL_INFO_SIZE_NON_STANDARD]byte
	}

	payload := &tracepointAcpiNonStandardPayload{}

	reader := bytes.NewReader(data.Info[:])
	err := binary.Read(reader, binary.LittleEndian, payload)
	if err != nil {
		return nil, fmt.Errorf("parse acpi non_standard: %w", err)
	}

	msg := payload.Msg[:]

	fruOff := payload.FruTxtOffset&0xffff - 56
	fruEnd := bytes.IndexByte(msg[fruOff:], 0)
	fru := string(msg[fruOff : int(fruOff)+fruEnd])

	rawData, _ := CopyFromOffset(payload.Msg[:], int(fruOff), int(payload.Len))

	errType := ErrTypeRecovPanic
	if payload.Sev < 2 {
		errType = ErrTypeCorrected
	}

	return &RasTracingData{
		Timestamp: data.Timestamp,
		Device:    "ACPI",
		Event:     "NON_STANDARD",
		ErrType:   errType,
		Info: fmt.Sprintf("severity: %d; "+
			"sec type:%x; FRU: %x%s; "+
			"data len:%d; raw data:% x",
			payload.Sev,
			payload.SecType,
			payload.FruID,
			fru,
			payload.Len,
			rawData,
		),
	}, nil
}

func buildRasAerTracerData(data *rasPerfEvent) (*RasTracingData, error) {
	type tracepointAerEventPayload struct {
		Pad            uint64
		DevNameOffset  uint32
		Status         uint32
		Severity       uint8
		TlpHeaderValid uint8
		Pattern        [2]uint8
		TlpHeader      [4]uint32
		Msg            [DETAIL_INFO_SIZE_AER]byte
	}

	payload := &tracepointAerEventPayload{}

	reader := bytes.NewReader(data.Info[:])
	err := binary.Read(reader, binary.LittleEndian, payload)
	if err != nil {
		return nil, fmt.Errorf("parse PCIe: %w", err)
	}

	msg := payload.Msg[:]
	devOff := payload.DevNameOffset&0xffff - 36
	devEnd := bytes.IndexByte(msg[devOff:], 0)
	dev := string(msg[devOff : int(devOff)+devEnd])

	var (
		errReason string
		errType   string
		tlpHeader string
	)

	if payload.Severity == 2 {
		errReason = pciErr(payload.Status, true)
		errType = ErrTypeCorrected
	} else {
		errReason = pciErr(payload.Status, false)

		errType = ErrTypeUncorrected
		if payload.Severity == 1 {
			errType = ErrTypeFatal
		}
	}

	tlpHeader = "not available"
	if payload.TlpHeaderValid != 0 {
		tlpHeader = fmt.Sprintf("{%#x,%#x,%#x,%#x}",
			payload.TlpHeader[0],
			payload.TlpHeader[1],
			payload.TlpHeader[2],
			payload.TlpHeader[3])
	}

	return &RasTracingData{
		Timestamp: data.Timestamp,
		Device:    fmt.Sprintf("PCIe %s", dev),
		Event:     "AER",
		ErrType:   errType,
		Info: fmt.Sprintf("PCIe Device: %s, Error: %s, Reason: %s, TLP Header: %s",
			dev, errType, errReason, tlpHeader,
		),
	}, nil
}

func (ras *rasTracing) Start(ctx context.Context) (err error) {
	b, err := bpf.LoadBpf(bpf.ThisBpfOBJ(), nil)
	if err != nil {
		return fmt.Errorf("load bpf: %w", err)
	}
	defer b.Close()

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	reader, err := b.AttachAndEventPipe(childCtx, "ras_event_map", 8192)
	if err != nil {
		return fmt.Errorf("attach and event pipe: %w", err)
	}
	defer reader.Close()

	thresholdCount, err = getThrInfo()
	if err != nil {
		return err
	}

	for {
		select {
		case <-childCtx.Done():
			return nil
		default:
			var (
				err        error
				eventData  rasPerfEvent
				tracerData *RasTracingData
			)

			if err := reader.ReadInto(&eventData); err != nil {
				return fmt.Errorf("read ras event: %w", err)
			}

			atomic.AddUint64(&ras.count, 1)

			switch eventData.Type {
			case HW_ERR_MCE:
				tracerData, err = buildRasMceTracerData(&eventData)
			case HW_ERR_EDAC:
				tracerData, err = buildRasEdacTracerData(&eventData)
			case HW_ERR_NON_STANDARD:
				tracerData, err = buildRasAcpiTracerData(&eventData)
			case HW_ERR_AER_EVENT:
				tracerData, err = buildRasAerTracerData(&eventData)
			default:
				return fmt.Errorf("hardware err type not supported")
			}

			if err != nil {
				return err
			}
			storage.Save("ras", "", time.Now(), tracerData)
		}
	}
}

func getThrInfo() (uint64, error) {
	file, err := os.Open(interruptsPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open interrupts: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "THR") {
			var nums []uint64
			var sum uint64

			for _, field := range strings.Fields(line) {
				if num, err := strconv.ParseUint(field, 10, 64); err == nil {
					nums = append(nums, num)
					sum += num
				}
			}

			if len(nums) == 0 {
				return 0, fmt.Errorf("failed to find nums")
			}
			return sum, nil
		}
	}
	return 0, fmt.Errorf("didn't find interrupts info")
}

func (ras *rasTracing) Update() ([]*metric.Data, error) {
	count, err := getThrInfo()
	if err != nil {
		return nil, err
	}

	if thresholdCount < count {
		delta := count - thresholdCount
		thresholdCount = count
		atomic.AddUint64(&ras.count, 1)

		tracerData := &RasTracingData{}

		tracerData.Device = "ACPI"
		tracerData.Event = "Threshold APIC interrupts"
		tracerData.ErrType = ErrTypeCorrected
		tracerData.Info = fmt.Sprintf("%d threshold interrupts occurred, totaling %d", delta, thresholdCount)

		storage.Save("ras", "", time.Now(), tracerData)
	}

	return []*metric.Data{
		metric.NewCounterData("hw_total", float64(atomic.LoadUint64(&ras.count)),
			"ras counter", nil),
	}, nil
}
