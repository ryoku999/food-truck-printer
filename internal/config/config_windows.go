package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ServiceName = "FoodTruckPrinter"

type Settings struct {
	URL        string `json:"apiUrl"`
	Secret     []byte `json:"protectedToken"`
	Simulation bool   `json:"simulation"`
}

var tokenPattern = regexp.MustCompile("^[a-fA-F0-9]{64}$")

func InstallDir() (string, error) {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "Food Truck Printer"), nil
}

func NormalizeURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("ingresa una URL de API válida, sin usuario, consulta ni fragmento")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	local := strings.EqualFold(host, "localhost") || ip != nil && ip.IsLoopback()
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && local) {
		return "", fmt.Errorf("usa HTTPS para la API remota; HTTP se admite en localhost")
	}
	return parsed.String(), nil
}

func New(apiURL, token string, simulation bool) (Settings, error) {
	normalized, err := NormalizeURL(apiURL)
	if err != nil {
		return Settings{}, err
	}
	token = strings.TrimSpace(token)
	if !tokenPattern.MatchString(token) {
		return Settings{}, fmt.Errorf("pega el token de 64 caracteres generado por NestJS")
	}
	input := []byte(token)
	source := windows.DataBlob{Size: uint32(len(input)), Data: &input[0]}
	var protected windows.DataBlob
	if err := windows.CryptProtectData(&source, nil, nil, 0, nil, windows.CRYPTPROTECT_LOCAL_MACHINE|windows.CRYPTPROTECT_UI_FORBIDDEN, &protected); err != nil {
		return Settings{}, fmt.Errorf("proteger el token: %w", err)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(protected.Data))))
	secret := append([]byte(nil), unsafe.Slice(protected.Data, int(protected.Size))...)
	return Settings{URL: normalized, Secret: secret, Simulation: simulation}, nil
}

func Load(filename string) (Settings, string, error) {
	var settings Settings
	raw, err := os.ReadFile(filename)
	if err != nil {
		return settings, "", err
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return settings, "", err
	}
	settings.URL, err = NormalizeURL(settings.URL)
	if err != nil {
		return settings, "", err
	}
	if len(settings.Secret) == 0 || len(settings.Secret) > 8192 {
		return settings, "", fmt.Errorf("token protegido inválido")
	}
	input := windows.DataBlob{Size: uint32(len(settings.Secret)), Data: &settings.Secret[0]}
	var output windows.DataBlob
	if err := windows.CryptUnprotectData(&input, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return settings, "", fmt.Errorf("leer token protegido: %w", err)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	token := string(unsafe.Slice(output.Data, int(output.Size)))
	if !tokenPattern.MatchString(token) {
		return settings, "", fmt.Errorf("token descifrado inválido")
	}
	return settings, token, nil
}

// PrivateDir restricts configuration, logs and journal to SYSTEM and administrators.
func PrivateDir(dir string) error {
	ptr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(ptr, &attrs); err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return err
	}
	flags, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return err
	}
	if flags&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || flags&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return fmt.Errorf("el directorio de datos debe ser una carpeta local normal")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func AtomicWrite(filename string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(filename), ".setup-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	from, err := windows.UTF16PtrFromString(f.Name())
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filename)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
