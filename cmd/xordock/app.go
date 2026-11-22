package main

// Polecenia zapytań dla aplikacji macOS (S007 §5): wynik JSON na stdout.
// Akcje zostają tekstowe; błąd to kod ≠ 0 i komunikat na stderr.

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"xordock/internal/fusemount"
	"xordock/internal/split"
	"xordock/internal/vault"
)

type jsonSpare struct {
	Root   string `json:"root"`
	Volume string `json:"volume"`
	Media  string `json:"media"`
}

type jsonBackup struct {
	Time     string `json:"time"`
	Point    string `json:"point"`
	Snapshot string `json:"snapshot"`
	Seq      uint64 `json:"seq"`
	Media    string `json:"media"`
}

// jsonCopy: kopia udziału A w iCloud Drive (S008).
type jsonCopy struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Path      string `json:"path"`
	Points    int    `json:"points"`
	LastPoint string `json:"lastPoint,omitempty"`
}

// readCopy opisuje kopię A z jej nagłówka i rekordów punktów (bez odczytu udziałów).
func readCopy(dir string) (jsonCopy, bool) {
	id, _, err := vault.ReadCopyID(dir)
	if err != nil {
		return jsonCopy{}, false
	}
	jc := jsonCopy{Name: filepath.Base(dir), ID: id.String(), Path: dir}
	pts, _ := vault.ReadPoints(dir, id)
	jc.Points = len(pts)
	if len(pts) > 0 {
		jc.LastPoint = ts(pts[0].Created)
	}
	return jc, true
}

type jsonVault struct {
	Name       string      `json:"name"`
	ID         string      `json:"id"`
	A          string      `json:"a"`
	Card       string      `json:"card,omitempty"`
	Spares     []jsonSpare `json:"spares"`
	Mounted    bool        `json:"mounted"`
	Mountpoint string      `json:"mountpoint"`
	Seq        uint64      `json:"seq"`
	Snapshots  int         `json:"snapshots"`
	LastBackup *jsonBackup `json:"lastBackup,omitempty"`
	Changes    int         `json:"changesSinceBackup"`
	BackupDue  bool        `json:"backupDue"`
	ACopy      *jsonCopy   `json:"aCopy,omitempty"`
	Error      string      `json:"error,omitempty"`
}

type jsonState struct {
	FileVault    bool        `json:"fileVault"`
	Masks        string      `json:"masks"`
	ICloud       bool        `json:"icloud"`
	Vaults       []jsonVault `json:"vaults"`
	OrphanCopies []jsonCopy  `json:"orphanCopies"`
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func vaultsDir() string { return filepath.Join(home(), "Library", "Application Support", "xordock") }

// volumeOf zwraca /Volumes/<nazwa> dla katalogu sejfu na karcie.
func volumeOf(root string) string { return filepath.Dir(filepath.Dir(root)) }

// collectVaults buduje stan wszystkich sejfów z katalogu udziałów A.
func collectVaults(dir, volumes string) []jsonVault {
	out := []jsonVault{}
	ms, _ := filepath.Glob(filepath.Join(dir, "*", "xordock.json"))
	sort.Strings(ms)
	for _, m := range ms {
		aRoot := filepath.Dir(m)
		name := filepath.Base(aRoot)
		jv := jsonVault{Name: name, A: aRoot, Spares: []jsonSpare{}, Mountpoint: mountpoint(name, "")}
		id, _, err := vault.ReadVaultID(aRoot)
		if err != nil {
			jv.Error = err.Error()
			out = append(out, jv)
			continue
		}
		jv.ID = id.String()
		if jc, ok := readCopy(aCopyDir(name)); ok && jc.ID == jv.ID {
			jv.ACopy = &jc
		}
		jv.Mounted = fusemount.IsMounted(jv.Mountpoint)
		for _, c := range vault.FindCards(volumes, id) {
			if c.Role == "" {
				if jv.Card == "" {
					jv.Card = c.Root
				}
			} else {
				jv.Spares = append(jv.Spares, jsonSpare{Root: c.Root, Volume: volumeOf(c.Root), Media: c.Media})
			}
		}
		if jv.Card == "" {
			jv.BackupDue = true
			out = append(out, jv)
			continue
		}
		s, err := vault.Peek(aRoot, jv.Card)
		if err != nil {
			jv.Error = err.Error()
			out = append(out, jv)
			continue
		}
		jv.Seq, jv.Snapshots, jv.Changes = s.Seq, s.Snapshots, s.Changes
		jv.BackupDue = s.BackupDue(time.Now())
		if s.LastBackup != nil {
			jv.LastBackup = &jsonBackup{Time: ts(s.LastBackup.Time), Point: s.LastBackup.Point,
				Snapshot: s.BackupSnap.ID, Seq: s.BackupSnap.Seq, Media: s.LastBackup.Media}
		}
		out = append(out, jv)
	}
	return out
}

// orphanCopies: kopie A w iCloud, dla których na tym Macu nie ma udziału A
// (np. po utracie SSD): punkt wyjścia do odzyskiwania w aplikacji.
func orphanCopies(dir string, vs []jsonVault) []jsonCopy {
	have := map[string]bool{}
	for _, v := range vs {
		have[v.ID] = true
	}
	out := []jsonCopy{}
	ms, _ := filepath.Glob(filepath.Join(dir, "*", "xordock.json"))
	sort.Strings(ms)
	for _, m := range ms {
		if jc, ok := readCopy(filepath.Dir(m)); ok && !have[jc.ID] {
			out = append(out, jc)
		}
	}
	return out
}

func fileVaultOn() bool {
	out, err := exec.Command("/usr/bin/fdesetup", "status").Output()
	return err == nil && strings.Contains(string(out), "On")
}

func cmdVaults(args []string) error {
	fl := flag.NewFlagSet("vaults", flag.ExitOnError)
	fl.Bool("json", true, "wynik JSON (jedyny format)")
	fl.Parse(args)
	vs := collectVaults(vaultsDir(), "/Volumes")
	return printJSON(jsonState{FileVault: fileVaultOn(), Masks: split.Kind(split.CSPRNG{}), ICloud: iCloudAvailable(),
		Vaults: vs, OrphanCopies: orphanCopies(filepath.Join(iCloudDrive(), "xordock"), vs)})
}

type jsonCardRef struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Role string `json:"role"`
	Root string `json:"root"`
}

type jsonVolume struct {
	Path     string        `json:"path"`
	Name     string        `json:"name"`
	FS       string        `json:"fs"`
	Free     uint64        `json:"free"`
	Total    uint64        `json:"total"`
	ReadOnly bool          `json:"readOnly"`
	System   bool          `json:"systemDisk"`
	Vaults   []jsonCardRef `json:"vaults"`
}

// wholeDisk: "/dev/disk3s1" → "disk3". Wstępna ochrona w UI przed wyborem
// woluminu z dysku systemowego; pełna kontrola fizycznego nośnika to S006.
func wholeDisk(dev string) string {
	d := strings.TrimPrefix(dev, "/dev/")
	if !strings.HasPrefix(d, "disk") {
		return ""
	}
	i := 4
	for i < len(d) && d[i] >= '0' && d[i] <= '9' {
		i++
	}
	return d[:i]
}

func cstr(b []int8) string {
	n := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		n = append(n, byte(c))
	}
	return string(n)
}

// collectVolumes zwraca zamontowane woluminy w katalogu volumes (punkty
// montowania, bez dowiązań i bez wolumenów samego xordocka).
func collectVolumes(volumes string) []jsonVolume {
	out := []jsonVolume{}
	var root syscall.Statfs_t
	sys := ""
	if syscall.Statfs("/", &root) == nil {
		sys = wholeDisk(cstr(root.Mntfromname[:]))
	}
	ents, _ := os.ReadDir(volumes)
	for _, e := range ents {
		p := filepath.Join(volumes, e.Name())
		if !e.IsDir() || strings.HasPrefix(e.Name(), "xordock-") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(p, &st) != nil || cstr(st.Mntonname[:]) != p {
			continue
		}
		v := jsonVolume{Path: p, Name: e.Name(), FS: cstr(st.Fstypename[:]),
			Free: st.Bavail * uint64(st.Bsize), Total: st.Blocks * uint64(st.Bsize),
			ReadOnly: st.Flags&0x1 != 0, Vaults: []jsonCardRef{}}
		v.System = sys != "" && wholeDisk(cstr(st.Mntfromname[:])) == sys
		for _, c := range vault.ListCards(p) {
			v.Vaults = append(v.Vaults, jsonCardRef{Name: c.Name, ID: c.Vault, Role: c.Role, Root: c.Root})
		}
		out = append(out, v)
	}
	return out
}

func cmdVolumes(args []string) error {
	fl := flag.NewFlagSet("volumes", flag.ExitOnError)
	fl.Bool("json", true, "wynik JSON (jedyny format)")
	fl.Parse(args)
	return printJSON(collectVolumes("/Volumes"))
}

type jsonPoint struct {
	ID       string `json:"id"`
	Seq      uint64 `json:"seq"`
	Snapshot string `json:"snapshot"`
	Created  string `json:"created"`
	Objects  int    `json:"objects"`
	Bytes    int64  `json:"bytes"`
}

type jsonPoints struct {
	Points   []jsonPoint `json:"points"`
	Rejected []string    `json:"rejected"`
}

func cmdPoints(args []string) error {
	fl := flag.NewFlagSet("points", flag.ExitOnError)
	var c common
	c.bind(fl)
	spareVol := fl.String("spare", "", "wolumen karty zapasowej (przy --from-icloud także roboczej)")
	fromICloud := fl.Bool("from-icloud", false, "udział A z kopii w iCloud Drive")
	fl.Bool("json", true, "wynik JSON (jedyny format)")
	fl.Parse(args)
	spare, err := spareRoot(*spareVol, c.name)
	if err != nil {
		return err
	}
	aSrc := c.aRoot()
	if *fromICloud {
		aSrc = aCopyDir(c.name)
	}
	r, err := vault.OpenRecovery(aSrc, spare)
	if err != nil {
		return err
	}
	defer r.Close()
	pts, bad := r.Points()
	out := jsonPoints{Points: []jsonPoint{}, Rejected: []string{}}
	for _, p := range pts {
		out.Points = append(out.Points, jsonPoint{ID: p.ID, Seq: p.Seq, Snapshot: p.Snapshot,
			Created: ts(p.Created), Objects: p.Objects, Bytes: p.Bytes})
	}
	out.Rejected = append(out.Rejected, bad...)
	return printJSON(out)
}
