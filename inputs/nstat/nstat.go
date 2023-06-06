package nstat

import (
	"bytes"
	"log"
	"os"
	"runtime"
	"strconv"

	"flashcat.cloud/categraf/config"
	"flashcat.cloud/categraf/inputs"
	"flashcat.cloud/categraf/types"
)

const inputName = "nstat"

var (
	zeroByte    = []byte("0")
	newLineByte = []byte("\n")
	colonByte   = []byte(":")
)

// default file paths
const (
	NetNetstat = "/net/netstat"
	NetSnmp    = "/net/snmp"
	NetSnmp6   = "/net/snmp6"
	NetProc    = "/proc"
)

// env variable names
const (
	EnvNetstat = "PROC_NET_NETSTAT"
	EnvSnmp    = "PROC_NET_SNMP"
	EnvSnmp6   = "PROC_NET_SNMP6"
	EnvRoot    = "PROC_ROOT"
)

type Nstat struct {
	ProcNetNetstat string `toml:"proc_net_netstat"`
	ProcNetSNMP    string `toml:"proc_net_snmp"`
	ProcNetSNMP6   string `toml:"proc_net_snmp6"`
	DumpZeros      bool   `toml:"dump_zeros"`
	config.PluginConfig
}

func init() {
	inputs.Add(inputName, func() inputs.Input {
		return &Nstat{}
	})
}

func (ns *Nstat) Clone() inputs.Input {
	return &Nstat{}
}

func (ns *Nstat) Name() string {
	return inputName
}

func (ns *Nstat) Gather(slist *types.SampleList) {
	if runtime.GOOS != "linux" {
		return
	}
	// load paths, get from env if config values are empty
	ns.loadPaths()

	netstat, err := os.ReadFile(ns.ProcNetNetstat)
	if err != nil {
		log.Println("E! failed to get netstat:", err)
		return
	}

	// collect netstat data
	ns.gatherNetstat(netstat, slist)

	// collect SNMP data
	snmp, err := os.ReadFile(ns.ProcNetSNMP)
	if err != nil {
		log.Println("E! failed to get snmp:", err)
		return
	}
	ns.gatherSNMP(snmp, slist)

	// collect SNMP6 data, if SNMP6 directory exists (IPv6 enabled)
	snmp6, err := os.ReadFile(ns.ProcNetSNMP6)
	if err == nil {
		ns.gatherSNMP6(snmp6, slist)
	} else if !os.IsNotExist(err) {
		log.Println("E! failed to get snmp6:", err)
		return
	}
}

func (ns *Nstat) gatherNetstat(data []byte, slist *types.SampleList) {
	metrics := ns.loadUglyTable(data)
	tags := map[string]string{
		"name": "netstat",
	}
	slist.PushSamples(inputName, metrics, tags)
}

func (ns *Nstat) gatherSNMP(data []byte, slist *types.SampleList) {
	metrics := ns.loadUglyTable(data)
	tags := map[string]string{
		"name": "snmp",
	}
	slist.PushSamples(inputName, metrics, tags)
}

func (ns *Nstat) gatherSNMP6(data []byte, slist *types.SampleList) {
	metrics := ns.loadGoodTable(data)
	tags := map[string]string{
		"name": "snmp6",
	}
	slist.PushSamples(inputName, metrics, tags)
}

// loadPaths can be used to read paths firstly from config
// if it is empty then try read from env variables
func (ns *Nstat) loadPaths() {
	if ns.ProcNetNetstat == "" {
		ns.ProcNetNetstat = proc(EnvNetstat, NetNetstat)
	}
	if ns.ProcNetSNMP == "" {
		ns.ProcNetSNMP = proc(EnvSnmp, NetSnmp)
	}
	if ns.ProcNetSNMP6 == "" {
		ns.ProcNetSNMP6 = proc(EnvSnmp6, NetSnmp6)
	}
}

// loadGoodTable can be used to parse string heap that
// headers and values are arranged in right order
func (ns *Nstat) loadGoodTable(table []byte) map[string]interface{} {
	entries := map[string]interface{}{}
	fields := bytes.Fields(table)
	var value int64
	var err error
	// iterate over two values each time
	// first value is header, second is value
	for i := 0; i < len(fields); i = i + 2 {
		// counter is zero
		if bytes.Equal(fields[i+1], zeroByte) {
			if !ns.DumpZeros {
				continue
			}

			entries[string(fields[i])] = int64(0)
			continue
		}
		// the counter is not zero, so parse it.
		value, err = strconv.ParseInt(string(fields[i+1]), 10, 64)
		if err == nil {
			entries[string(fields[i])] = value
		}
	}
	return entries
}

// loadUglyTable can be used to parse string heap that
// the headers and values are splitted with a newline
func (ns *Nstat) loadUglyTable(table []byte) map[string]interface{} {
	entries := map[string]interface{}{}
	// split the lines by newline
	lines := bytes.Split(table, newLineByte)
	var value int64
	var err error
	// iterate over lines, take 2 lines each time
	// first line contains header names
	// second line contains values
	for i := 0; i < len(lines); i = i + 2 {
		if len(lines[i]) == 0 {
			continue
		}
		headers := bytes.Fields(lines[i])
		prefix := bytes.TrimSuffix(headers[0], colonByte)
		metrics := bytes.Fields(lines[i+1])

		for j := 1; j < len(headers); j++ {
			// counter is zero
			if bytes.Equal(metrics[j], zeroByte) {
				if !ns.DumpZeros {
					continue
				}

				entries[string(append(prefix, headers[j]...))] = int64(0)
				continue
			}
			// the counter is not zero, so parse it.
			value, err = strconv.ParseInt(string(metrics[j]), 10, 64)
			if err == nil {
				entries[string(append(prefix, headers[j]...))] = value
			}
		}
	}
	return entries
}

// proc can be used to read file paths from env
func proc(env, path string) string {
	// try to read full file path
	if p := os.Getenv(env); p != "" {
		return p
	}
	// try to read root path, or use default root path
	root := os.Getenv(EnvRoot)
	if root == "" {
		root = NetProc
	}
	return root + path
}
