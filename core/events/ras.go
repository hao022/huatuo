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
	"encoding/json"
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

// rasTracing holds per-instance state; all fields are accessed via atomic ops
// so that Start (goroutine A) and Update (goroutine B, Prometheus scrape) do
// not race.
type rasTracing struct {
	count          uint64 // total RAS events observed
	thresholdCount uint64 // MCE threshold-interrupt baseline set in Start
}

// Hardware error type identifiers — must stay in sync with bpf/ras.c.
const (
	HW_ERR_MCE       = 0
	HW_ERR_EDAC      = 1
	HW_ERR_ACPI_GHES = 2
	HW_ERR_PCIE_AER  = 3
)

// Error severity labels written into RasTracingData.ErrType.
const (
	ErrTypeCorrected              = "Corrected"
	ErrTypeUncorrectedRecoverable = "UncorrectedRecoverable"
	ErrTypeUncorrectedDeferred    = "UncorrectedDeferred"
	ErrTypeUncorrectedFatal       = "UncorrectedFatal"
	ErrTypeInfo                   = "Info"
	ErrTypeUnknown                = "unknown"
)

// The dynamic_array info always lives at the very end of each tracepoint
// event struct.  We read the full 512-byte buffer so the kernel never needs
// to truncate the payload.
//
// Fixed-portion byte counts (the static fields, excluding the 4-byte
// __data_loc field that precedes the dynamic area):
//
//	trace_event_raw_mc_event:           64 − 4 = 60
//	trace_event_raw_non_standard_event: 60 − 4 = 56
//	trace_event_raw_aer_event:          40 − 4 = 36
const (
	RAS_PERFEVENT_INFO_SIZE = 512
	DETAIL_INFO_SIZE_EDAC   = RAS_PERFEVENT_INFO_SIZE - 60
	DETAIL_INFO_SIZE_ACPI   = RAS_PERFEVENT_INFO_SIZE - 56
	DETAIL_INFO_SIZE_AER    = RAS_PERFEVENT_INFO_SIZE - 36
)

// rasEvent mirrors the BPF-side struct event layout.
type rasEvent struct {
	Type      uint32
	Pad0      uint32
	Timestamp uint64
	Info      [RAS_PERFEVENT_INFO_SIZE]byte
}

// RasTracingData is the structured record persisted by storage.Save.
type RasTracingData struct {
	Device    string `json:"dev"`
	Event     string `json:"event"`
	ErrType   string `json:"type"`
	Timestamp uint64 `json:"timestamp"`
	Info      string `json:"info"`
}

var interruptsPath = "/proc/interrupts"

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

func decodePayload[T any](info []byte) (*T, error) {
	var payload T
	if err := binary.Read(bytes.NewReader(info), binary.LittleEndian, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func cstring(buf []byte, rawOffset, base uint32) string {
	absOff := rawOffset & 0xffff
	if absOff < base {
		return ""
	}
	off := int(absOff - base)
	if off >= len(buf) {
		return ""
	}
	if end := bytes.IndexByte(buf[off:], 0); end >= 0 {
		return string(buf[off : off+end])
	}
	return string(buf[off:])
}

// Bank's MCi_STATUS MSR
//
// #define MCI_STATUS_DEFERRED     BIT_ULL(44)  /* uncorrected error, deferred exception */
// #define MCI_STATUS_UC           BIT_ULL(61)  /* uncorrected error */

const (
	MCI_STATUS_DEFERRED = 1 << 44
	MCI_STATUS_UC       = 1 << 61
)

func mceErrType(status uint64) string {
	if status&MCI_STATUS_DEFERRED != 0 {
		return ErrTypeUncorrectedDeferred
	}

	if status&MCI_STATUS_UC != 0 {
		return ErrTypeUncorrectedRecoverable
	}

	return ErrTypeCorrected
}

// copy from linux kernel include/linux/edac.h
//
/**
 * enum hw_event_mc_err_type - type of the detected error
 *
 * @HW_EVENT_ERR_CORRECTED:     Corrected Error - Indicates that an ECC
 *                              corrected error was detected
 * @HW_EVENT_ERR_UNCORRECTED:   Uncorrected Error - Indicates an error that
 *                              can't be corrected by ECC, but it is not
 *                              fatal (maybe it is on an unused memory area,
 *                              or the memory controller could recover from
 *                              it for example, by re-trying the operation).
 * @HW_EVENT_ERR_DEFERRED:      Deferred Error - Indicates an uncorrectable
 *                              error whose handling is not urgent. This could
 *                              be due to hardware data poisoning where the
 *                              system can continue operation until the poisoned
 *                              data is consumed. Preemptive measures may also
 *                              be taken, e.g. offlining pages, etc.
 * @HW_EVENT_ERR_FATAL:         Fatal Error - Uncorrected error that could not
 *                              be recovered.
 * @HW_EVENT_ERR_INFO:          Informational - The CPER spec defines a forth
 *                              type of error: informational logs.
 */

/**
 * enum hw_event_mc_err_type {
 *        HW_EVENT_ERR_CORRECTED,
 *        HW_EVENT_ERR_UNCORRECTED,
 *        HW_EVENT_ERR_DEFERRED,
 *        HW_EVENT_ERR_FATAL,
 *        HW_EVENT_ERR_INFO,
 * };
 */
func edacErrType(errType uint32) string {
	switch errType {
	case 0x0:
		return ErrTypeCorrected
	case 0x01:
		return ErrTypeUncorrectedRecoverable
	case 0x02:
		return ErrTypeUncorrectedDeferred
	case 0x03:
		return ErrTypeUncorrectedFatal
	case 0x04:
		return ErrTypeInfo
	default:
		return ErrTypeUnknown
	}
}

// acpiErrType maps an ACPI non-standard event severity to an error type.
//
// linux kernel include/acpi/ghes.h
//
//	enum {
//	        GHES_SEV_NO = 0x0,
//	        GHES_SEV_CORRECTED = 0x1,
//	        GHES_SEV_RECOVERABLE = 0x2,
//	        GHES_SEV_PANIC = 0x3,
//	};
//
// ghes_edac_report_mem_error()
//
// GHES_SEV_CORRECTED - HW_EVENT_ERR_CORRECTED
// GHES_SEV_RECOVERABLE - HW_EVENT_ERR_UNCORRECTED
// GHES_SEV_PANIC - HW_EVENT_ERR_FATAL
// GHES_SEV_NO - HW_EVENT_ERR_INFO
func acpiErrType(sev uint8) string {
	switch sev {
	case 0x0:
		return ErrTypeInfo
	case 0x1:
		return ErrTypeCorrected
	case 0x2:
		return ErrTypeUncorrectedRecoverable
	case 0x3:
		return ErrTypeUncorrectedFatal
	default:
		return ErrTypeUnknown
	}
}

// aerErrType maps a PCIe AER severity value to an error type.
//
// linux kernel include/linux/aer.h
//
// AER_CORRECTABLE 2
// AER_FATAL 1
// AER_NONFATAL 0
func aerErrType(severity uint8) string {
	switch severity {
	case 2:
		return ErrTypeCorrected
	case 1:
		return ErrTypeUncorrectedFatal
	case 0:
		return ErrTypeUncorrectedRecoverable
	default:
		return ErrTypeUnknown
	}
}

// ---------------------------------------------------------------------------
// PCI AER error status bits (PCIe Base Spec §7.8.4)
// ---------------------------------------------------------------------------

// Correctable error status bits.
const (
	PciErrCorRcvr     uint32 = 0x00000001 /* Receiver Error */
	PciErrCorBadTlp   uint32 = 0x00000040 /* Bad TLP */
	PciErrCorBadDllp  uint32 = 0x00000080 /* Bad DLLP */
	PciErrCorRepRoll  uint32 = 0x00000100 /* REPLAY_NUM Rollover */
	PciErrCorRepTimer uint32 = 0x00001000 /* Replay Timer Timeout */
	PciErrCorAdvNfat  uint32 = 0x00002000 /* Advisory Non-Fatal */
	PciErrCorInternal uint32 = 0x00004000 /* Corrected Internal */
	PciErrCorLogOver  uint32 = 0x00008000 /* Header Log Overflow */
)

// Uncorrectable error status bits.
const (
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
	PciErrUncEcrc      uint32 = 0x00080000 /* ECRC Error */
	PciErrUncUnsup     uint32 = 0x00100000 /* Unsupported Request */
	PciErrUncAscv      uint32 = 0x00200000 /* ACS Violation */
	PciErrUncIntn      uint32 = 0x00400000 /* Uncorrectable Internal */
	PciErrUncMcptlp    uint32 = 0x00800000 /* MC Blocked TLP */
	PciErrUncAtomeg    uint32 = 0x01000000 /* AtomicOp Egress Blocked */
	PciErrUncTlpPre    uint32 = 0x02000000 /* TLP Prefix Blocked */
)

var aerCorrectableErrors = map[uint32]string{
	PciErrCorRcvr:     "Receiver Error",
	PciErrCorBadTlp:   "Bad TLP",
	PciErrCorBadDllp:  "Bad DLLP",
	PciErrCorRepRoll:  "REPLAY_NUM Rollover",
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

func pciErrReason(status uint32, correctable bool) string {
	m := aerUncorrectableErrors
	if correctable {
		m = aerCorrectableErrors
	}
	if name, ok := m[status]; ok {
		return name
	}
	return "unknown"
}

func newRasTracingData(ev *rasEvent, device, event, errType string, info any) (*RasTracingData, error) {
	b, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("marshal %s info: %w", event, err)
	}
	return &RasTracingData{
		Timestamp: ev.Timestamp,
		Device:    device,
		Event:     event,
		ErrType:   errType,
		Info:      string(b),
	}, nil
}

// ---------------------------------------------------------------------------
// Per-event-type builder functions
// ---------------------------------------------------------------------------

func buildRasMceTracerData(data *rasEvent) (*RasTracingData, error) {
	// tracepointMcePayload mirrors struct trace_event_raw_mce_record.
	// https://git.kernel.org/pub/scm/linux/kernel/git/netdev/net-next.git/tree/arch/x86/include/uapi/asm/mce.h
	type tracepointMcePayload struct {
		Pad       uint64 `json:"-"`
		Mcgcap    uint64 `json:"mcg_cpu_cap"`
		McgStatus uint64 `json:"mcg_msr_status"`
		Status    uint64 `json:"banks_msr_status"`
		Addr      uint64 `json:"banks_msr_addr"`
		Misc      uint64 `json:"banks_msr_misc"`
		Synd      uint64 `json:"mca_synd_msr"`
		Ipid      uint64 `json:"mca_ipid_msr"`
		Ip        uint64 `json:"instr_pointer"`
		Tsc       uint64 `json:"tsc_timestamp"`
		Walltime  uint64 `json:"walltime"`
		Cpu       uint32 `json:"cpu"`
		Cpuid     uint32 `json:"cpuid"`
		Apicid    uint32 `json:"apicid"`
		Socketid  uint32 `json:"socketid"`
		Cs        uint8  `json:"code_seg"`
		Bank      uint8  `json:"bank"`
		Cpuvendor uint8  `json:"cpuvendor"`
	}

	payload, err := decodePayload[tracepointMcePayload](data.Info[:])
	if err != nil {
		return nil, fmt.Errorf("parse MCE payload: %w", err)
	}
	return newRasTracingData(data, "CPU/MEM", "MCE", mceErrType(payload.Status), payload)
}

func buildRasEdacTracerData(data *rasEvent) (*RasTracingData, error) {
	// tracepointEdacPayload mirrors struct trace_event_raw_mc_event.
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

	payload, err := decodePayload[tracepointEdacPayload](data.Info[:])
	if err != nil {
		return nil, fmt.Errorf("parse EDAC payload: %w", err)
	}

	const edacBase uint32 = 60
	dyn := payload.MsgDetail[:]
	return newRasTracingData(data, "MEM", "EDAC", edacErrType(payload.ErrType), struct {
		ErrCount uint16 `json:"err_count"`
		ErrType  string `json:"err_type"`
		Msg      string `json:"err_msg"`
		Label    string `json:"label"`
		McIndex  uint8  `json:"mc_index"`
		TopLayer int8   `json:"top_layer"`
		MidLayer int8   `json:"mid_layer"`
		LowLayer int8   `json:"low_layer"`
		Addr     uint64 `json:"addr"`
		Grain    uint64 `json:"grain"`
		Syndrome uint64 `json:"syndrome"`
		Driver   string `json:"driver"`
	}{
		ErrCount: payload.ErrCount,
		ErrType:  edacErrType(payload.ErrType),
		Msg:      cstring(dyn, payload.ErrorMsgOffset, edacBase),
		Label:    cstring(dyn, payload.LabelOffset, edacBase),
		McIndex:  payload.McIndex,
		TopLayer: payload.TopLayer,
		MidLayer: payload.MidLayer,
		LowLayer: payload.LowLayer,
		Addr:     payload.Addr,
		Grain:    uint64(1) << payload.GrainBits,
		Syndrome: payload.Syndrome,
		Driver:   cstring(dyn, payload.DriverDetail, edacBase),
	})
}

func buildRasAcpiTracerData(data *rasEvent) (*RasTracingData, error) {
	// tracepointAcpiNonStandardPayload mirrors
	// struct trace_event_raw_non_standard_event.
	type tracepointAcpiNonStandardPayload struct {
		Pad          uint64
		SecType      [16]uint8
		FruID        [16]uint8
		FruTxtOffset uint32
		Sev          uint8
		Pattern      [3]uint8
		Len          uint32
		BufOffset    uint32
		Msg          [DETAIL_INFO_SIZE_ACPI]byte
	}

	payload, err := decodePayload[tracepointAcpiNonStandardPayload](data.Info[:])
	if err != nil {
		return nil, fmt.Errorf("parse ACPI non-standard payload: %w", err)
	}

	const nonStandardBase uint32 = 56
	fru := cstring(payload.Msg[:], payload.FruTxtOffset, nonStandardBase)

	// Extract raw bytes at the FRU text location for the hex dump.
	var rawData []byte
	if absOff := payload.FruTxtOffset & 0xffff; absOff >= nonStandardBase {
		rawData = bytes.Clone(payload.Msg[absOff-nonStandardBase : absOff-nonStandardBase+payload.Len])
	}

	return newRasTracingData(data, "ACPI", "NON_STANDARD", acpiErrType(payload.Sev), struct {
		Severity uint8  `json:"severity"`
		SecType  string `json:"sec_type"`
		FruID    string `json:"fru_id"`
		FruText  string `json:"fru_text"`
		DataLen  uint32 `json:"data_len"`
		RawData  string `json:"raw_data"`
	}{
		Severity: payload.Sev,
		SecType:  fmt.Sprintf("%x", payload.SecType),
		FruID:    fmt.Sprintf("%x", payload.FruID),
		FruText:  fru,
		DataLen:  payload.Len,
		RawData:  fmt.Sprintf("% x", rawData),
	})
}

func buildRasAerTracerData(data *rasEvent) (*RasTracingData, error) {
	// tracepointAerEventPayload mirrors struct trace_event_raw_aer_event.
	type tracepointAerEventPayload struct {
		Pad            uint64
		DevNameOffset  uint32 // __data_loc_dev_name
		Status         uint32
		Severity       uint8
		TlpHeaderValid uint8
		Pattern        [2]uint8
		TlpHeader      [4]uint32
		Msg            [DETAIL_INFO_SIZE_AER]byte
	}

	payload, err := decodePayload[tracepointAerEventPayload](data.Info[:])
	if err != nil {
		return nil, fmt.Errorf("parse PCIe AER payload: %w", err)
	}

	const aerBase uint32 = 36
	dev := cstring(payload.Msg[:], payload.DevNameOffset, aerBase)

	errType := aerErrType(payload.Severity)
	errReason := pciErrReason(payload.Status, payload.Severity == 2)

	tlpHeader := "not available"
	if payload.TlpHeaderValid != 0 {
		tlpHeader = fmt.Sprintf("{%#x,%#x,%#x,%#x}",
			payload.TlpHeader[0], payload.TlpHeader[1],
			payload.TlpHeader[2], payload.TlpHeader[3])
	}

	return newRasTracingData(data, "PCIe "+dev, "AER", errType, struct {
		DevName   string `json:"dev_name"`
		ErrType   string `json:"err_type"`
		ErrReason string `json:"err_reason"`
		TlpHeader string `json:"tlp_header"`
	}{
		DevName:   dev,
		ErrType:   errType,
		ErrReason: errReason,
		TlpHeader: tlpHeader,
	})
}

func dispatchRasTracerData(data *rasEvent) (*RasTracingData, error) {
	switch data.Type {
	case HW_ERR_MCE:
		return buildRasMceTracerData(data)
	case HW_ERR_EDAC:
		return buildRasEdacTracerData(data)
	case HW_ERR_ACPI_GHES:
		return buildRasAcpiTracerData(data)
	case HW_ERR_PCIE_AER:
		return buildRasAerTracerData(data)
	default:
		return nil, fmt.Errorf("unsupported hardware error type %d", data.Type)
	}
}

// ---------------------------------------------------------------------------
// ITracingEvent implementation
// ---------------------------------------------------------------------------

func (ras *rasTracing) Start(ctx context.Context) error {
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

	// Establish the THR-interrupt baseline so Update only reports deltas.
	// getThrInfo may legitimately fail on systems without MCE threshold
	// interrupts (VMs, ARM); treat 0 as the baseline in that case.
	initial, err := getThrInfo()
	if err != nil {
		initial = 0
	}
	atomic.StoreUint64(&ras.thresholdCount, initial)

	for {
		select {
		case <-childCtx.Done():
			return nil
		default:
			var eventData rasEvent
			if err := reader.ReadInto(&eventData); err != nil {
				return fmt.Errorf("read ras event: %w", err)
			}

			atomic.AddUint64(&ras.count, 1)

			tracerData, err := dispatchRasTracerData(&eventData)
			if err != nil {
				// Unknown or malformed event — skip rather than aborting
				// the entire tracer.  Callers can observe the raw count
				// metric to detect repeated parse failures.
				continue
			}

			storage.Save("ras", "", time.Now(), tracerData)
		}
	}
}

// ---------------------------------------------------------------------------
// Collector implementation
// ---------------------------------------------------------------------------

func (ras *rasTracing) Update() ([]*metric.Data, error) {
	current, err := getThrInfo()
	if err != nil {
		return nil, err
	}

	prev := atomic.LoadUint64(&ras.thresholdCount)
	if prev < current {
		delta := current - prev
		atomic.StoreUint64(&ras.thresholdCount, current)
		atomic.AddUint64(&ras.count, 1)

		storage.Save("ras", "", time.Now(), &RasTracingData{
			Device:  "ACPI",
			Event:   "Threshold APIC interrupts",
			ErrType: ErrTypeCorrected,
			Info:    fmt.Sprintf("%d threshold interrupts occurred, totaling %d", delta, current),
		})
	}

	return []*metric.Data{
		metric.NewCounterData("hw_total", float64(atomic.LoadUint64(&ras.count)),
			"ras counter", nil),
	}, nil
}

// ---------------------------------------------------------------------------
// /proc/interrupts helper
// ---------------------------------------------------------------------------

// getThrInfo reads /proc/interrupts and returns the sum of per-CPU counts for
// the THR (MCE threshold) interrupt line.
func getThrInfo() (uint64, error) {
	file, err := os.Open(interruptsPath)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", interruptsPath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "THR") {
			continue
		}

		var sum uint64
		var count int
		for _, field := range strings.Fields(line) {
			if n, err := strconv.ParseUint(field, 10, 64); err == nil {
				sum += n
				count++
			}
		}
		if count == 0 {
			return 0, fmt.Errorf("no numeric counts in THR interrupt line")
		}
		return sum, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan %s: %w", interruptsPath, err)
	}
	return 0, fmt.Errorf("THR line not found in %s", interruptsPath)
}
