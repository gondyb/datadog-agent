// +build windows

package checks

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/shirou/w32"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	process "github.com/DataDog/gopsutil/process"
)

var (
	modpsapi                  = windows.NewLazyDLL("psapi.dll")
	modkernel                 = windows.NewLazyDLL("kernel32.dll")
	procGetProcessMemoryInfo  = modpsapi.NewProc("GetProcessMemoryInfo")
	procGetProcessHandleCount = modkernel.NewProc("GetProcessHandleCount")
	procGetProcessIoCounters  = modkernel.NewProc("GetProcessIoCounters")
)

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

func getAllProcesses(probe procutil.Probe) (map[int32]*procutil.Process, error) {
	return probe.ProcessesByPID(time.Now())
}

func getAllProcStats(probe procutil.Probe, pids []int32) (map[int32]*procutil.Stats, error) {
	return probe.StatsForPIDs(pids, time.Now())
}

func getProcessMemoryInfo(h windows.Handle, mem *process.PROCESS_MEMORY_COUNTERS) (err error) {
	r1, _, e1 := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(mem)), uintptr(unsafe.Sizeof(*mem)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getProcessHandleCount(h windows.Handle, count *uint32) (err error) {
	r1, _, e1 := procGetProcessHandleCount.Call(uintptr(h), uintptr(unsafe.Pointer(count)))
	if r1 == 0 {
		return e1
	}
	return nil
}

func getProcessIoCounters(h windows.Handle, counters *IO_COUNTERS) (err error) {
	r1, _, e1 := procGetProcessIoCounters.Call(uintptr(h), uintptr(unsafe.Pointer(counters)))
	if r1 == 0 {
		return e1
	}
	return nil
}

type legacyWindowsProbe struct {
	cachedProcesses map[uint32]*cachedProcess
}

func newLegacyWindowsProbe() procutil.Probe {
	return &legacyWindowsProbe{
		cachedProcesses: map[uint32]*cachedProcess{},
	}
}

func (p *legacyWindowsProbe) Close() {}

func (p *legacyWindowsProbe) StatsForPIDs(pids []int32, now time.Time) (map[int32]*procutil.Stats, error) {
	procs, err := p.ProcessesByPID(now)
	if err != nil {
		return nil, err
	}
	stats := make(map[int32]*procutil.Stats, len(procs))
	for pid, proc := range procs {
		stats[pid] = proc.Stats
	}
	return stats, nil
}

// StatsWithPermByPID is currently not implemented in non-linux environments
func (p *legacyWindowsProbe) StatsWithPermByPID(pids []int32) (map[int32]*procutil.StatsWithPerm, error) {
	return nil, fmt.Errorf("legacyWindowsProbe: StatsWithPermByPID is not implemented")
}

func (p *legacyWindowsProbe) ProcessesByPID(now time.Time) (map[int32]*procutil.Process, error) {
	// make sure we get the consistent snapshot by using the same OS thread
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	allProcsSnap := w32.CreateToolhelp32Snapshot(w32.TH32CS_SNAPPROCESS, 0)
	if allProcsSnap == 0 {
		return nil, windows.GetLastError()
	}
	procs := make(map[int32]*procutil.Process)

	defer w32.CloseHandle(allProcsSnap)
	var pe32 w32.PROCESSENTRY32
	pe32.DwSize = uint32(unsafe.Sizeof(pe32))

	knownPids := make(map[uint32]struct{})
	for pid := range p.cachedProcesses {
		knownPids[pid] = struct{}{}
	}

	for success := w32.Process32First(allProcsSnap, &pe32); success; success = w32.Process32Next(allProcsSnap, &pe32) {
		pid := pe32.Th32ProcessID
		ppid := pe32.Th32ParentProcessID

		if pid == 0 {
			// this is the "system idle process".  We'll never be able to open it,
			// which will cause us to thrash WMI once per check, which we don't
			// want to do.
			continue
		}
		cp, ok := p.cachedProcesses[pid]
		if !ok {
			// wasn't already in the map.
			cp = &cachedProcess{}

			if err := cp.fillFromProcEntry(&pe32); err != nil {
				log.Debugf("could not fill Win32 process information for pid %v %v", pid, err)
				continue
			}
			p.cachedProcesses[pid] = cp
		} else {
			var err error
			if cp.procHandle, err = procutil.OpenProcessHandle(int32(pe32.Th32ProcessID)); err != nil {
				log.Debugf("Could not reopen process handle for pid %v %v", pid, err)
				continue
			}
		}
		defer cp.close()

		procHandle := cp.procHandle

		var CPU windows.Rusage
		if err := windows.GetProcessTimes(procHandle, &CPU.CreationTime, &CPU.ExitTime, &CPU.KernelTime, &CPU.UserTime); err != nil {
			log.Debugf("Could not get process times for %v %v", pid, err)
			continue
		}

		var handleCount uint32
		if err := getProcessHandleCount(procHandle, &handleCount); err != nil {
			log.Debugf("could not get handle count for %v %v", pid, err)
			continue
		}

		var pmemcounter process.PROCESS_MEMORY_COUNTERS
		if err := getProcessMemoryInfo(procHandle, &pmemcounter); err != nil {
			log.Debugf("could not get memory info for %v %v", pid, err)
			continue
		}

		// shell out to getprocessiocounters for io stats
		var ioCounters IO_COUNTERS
		if err := getProcessIoCounters(procHandle, &ioCounters); err != nil {
			log.Debugf("could not get IO Counters for %v %v", pid, err)
			continue
		}
		ctime := CPU.CreationTime.Nanoseconds() / 1000000

		utime := float64((int64(CPU.UserTime.HighDateTime) << 32) | int64(CPU.UserTime.LowDateTime))
		stime := float64((int64(CPU.KernelTime.HighDateTime) << 32) | int64(CPU.KernelTime.LowDateTime))

		delete(knownPids, pid)
		procs[int32(pid)] = &procutil.Process{
			Pid:     int32(pid),
			Ppid:    int32(ppid),
			Cmdline: cp.parsedArgs,
			Stats: &procutil.Stats{
				CreateTime:  ctime,
				OpenFdCount: int32(handleCount),
				NumThreads:  int32(pe32.CntThreads),
				CPUTime: &procutil.CPUTimesStat{
					User:      utime,
					System:    stime,
					Timestamp: time.Now().UnixNano(),
				},
				MemInfo: &procutil.MemoryInfoStat{
					RSS:  uint64(pmemcounter.WorkingSetSize),
					VMS:  uint64(pmemcounter.QuotaPagedPoolUsage),
					Swap: 0,
				},
				IOStat: &procutil.IOCountersStat{
					ReadCount:  int64(ioCounters.ReadOperationCount),
					WriteCount: int64(ioCounters.WriteOperationCount),
					ReadBytes:  int64(ioCounters.ReadTransferCount),
					WriteBytes: int64(ioCounters.WriteTransferCount),
				},
				CtxSwitches: &procutil.NumCtxSwitchesStat{},
			},

			Exe:      cp.executablePath,
			Username: cp.userName,
		}
	}
	for pid := range knownPids {
		cp := p.cachedProcesses[pid]
		log.Debugf("removing process %v %v", pid, cp.executablePath)
		delete(p.cachedProcesses, pid)
	}

	return procs, nil
}

type cachedProcess struct {
	userName       string
	executablePath string
	commandLine    string
	procHandle     windows.Handle
	parsedArgs     []string
}

func (cp *cachedProcess) fillFromProcEntry(pe32 *w32.PROCESSENTRY32) (err error) {
	cp.procHandle, err = procutil.OpenProcessHandle(int32(pe32.Th32ProcessID))
	if err != nil {
		return err
	}
	var usererr error
	cp.userName, usererr = procutil.GetUsernameForProcess(cp.procHandle)
	if usererr != nil {
		log.Debugf("Couldn't get process username %v %v", pe32.Th32ProcessID, err)
	}
	var cmderr error
	cp.executablePath = winutil.ConvertWindowsString16(pe32.SzExeFile[:])
	commandParams, cmderr := winutil.GetCommandParamsForProcess(cp.procHandle, false)
	if cmderr != nil {
		log.Debugf("Error retrieving full command line %v", cmderr)
		cp.commandLine = cp.executablePath
	} else {
		cp.commandLine = commandParams.CmdLine
	}

	cp.parsedArgs = procutil.ParseCmdLineArgs(cp.commandLine)
	return
}

func (cp *cachedProcess) close() {
	if cp.procHandle != windows.Handle(0) {
		windows.CloseHandle(cp.procHandle)
		cp.procHandle = windows.Handle(0)
	}
	return
}
