package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPEVerificationAndSigner(t *testing.T) {
	// 1. Test Windows Catalog Verification (System binary in System32)
	systemPath := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	if fileExists(systemPath) {
		isCatalogSigned := verifyCatalogSig(systemPath)
		t.Logf("%s catalog signed: %v", systemPath, isCatalogSigned)
		info := lookupInfo(systemPath)
		t.Logf("%s lookupInfo: Exists=%v, Sig=%s, Signer=%q, Company=%q",
			systemPath, info.Exists, info.Sig, info.Signer, info.Company)
		if !info.Exists {
			t.Errorf("Expected %s to exist", systemPath)
		}
		if info.Sig != "valid" && info.Sig != "catalog" {
			t.Errorf("Expected cmd.exe to be signed (catalog or valid), got: %s", info.Sig)
		}
	}

	// 2. Test Embedded Authenticode Signature on signed app if available (e.g. Chrome, Edge, or Windows Defender)
	candidatePaths := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Windows Defender", "MpCmdRun.exe"),
	}

	var testedApp string
	for _, p := range candidatePaths {
		if fileExists(p) {
			signer, expired := getCertSigner(p)
			sigStatus := verifySig(p)
			t.Logf("Tested %s -> Sig: %s, Signer: %q, Expired: %v", p, sigStatus, signer, expired)
			if sigStatus == "valid" && signer == "" {
				t.Errorf("Expected valid signature on %s to have a subject signer name", p)
			}
			testedApp = p
			break
		}
	}
	if testedApp == "" {
		t.Log("Skipping candidate app test: none of Chrome, Edge, or MpCmdRun found in default locations")
	}

	// 3. Test version strings extraction
	kernel32 := filepath.Join(os.Getenv("SystemRoot"), "System32", "kernel32.dll")
	comp, prod := versionStrings(kernel32)
	t.Logf("kernel32.dll -> Company: %q, Product: %q", comp, prod)
	if comp == "" {
		t.Errorf("Expected CompanyName for kernel32.dll, got empty string")
	}

	// 4. Test SHA-256 calculation
	hash := calcSHA256(systemPath)
	if len(hash) != 64 {
		t.Errorf("Expected 64-char hex SHA256, got: %q", hash)
	}
}
