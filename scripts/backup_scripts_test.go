package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDBBackupCronExecsExplicitCommand(t *testing.T) {
	scriptsDir := mustGetwd(t)
	tempDir := t.TempDir()

	helperPath := filepath.Join(tempDir, "echo-args.sh")
	writeExecutable(t, helperPath, "#!/bin/sh\nprintf 'ok:%s\\n' \"$1\"\n")

	cmd := exec.Command("sh", filepath.Join(scriptsDir, "db-backup-cron.sh"), helperPath, "hello")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("db-backup-cron.sh returned error: %v\n%s", err, output)
	}

	if got := string(output); got != "ok:hello\n" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestDBRestoreListOutputsAvailableBackups(t *testing.T) {
	scriptsDir := mustGetwd(t)
	backupDir := t.TempDir()

	writeFile(t, filepath.Join(backupDir, "pentaract_20260327_184054.sql.gz"), "backup-1")
	writeFile(t, filepath.Join(backupDir, "pentaract_20260326_184054.sql.gz"), "backup-2")

	cmd := exec.Command("sh", filepath.Join(scriptsDir, "db-restore.sh"), "--list")
	cmd.Env = append(os.Environ(), "BACKUP_DIR="+backupDir)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("db-restore.sh --list returned error: %v\n%s", err, output)
	}

	got := string(output)
	if !strings.Contains(got, "pentaract_20260327_184054.sql.gz") {
		t.Fatalf("expected latest backup in output, got %q", got)
	}
	if !strings.Contains(got, "pentaract_20260326_184054.sql.gz") {
		t.Fatalf("expected older backup in output, got %q", got)
	}
}

func TestDBRestoreAcceptsHostAbsolutePath(t *testing.T) {
	scriptsDir := mustGetwd(t)
	backupDir := t.TempDir()
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gunzip.log")

	backupName := "pentaract_20260327_184054.sql.gz"
	resolvedBackupPath := filepath.Join(backupDir, backupName)
	writeFile(t, resolvedBackupPath, "compressed-backup")

	writeExecutable(t, filepath.Join(binDir, "gunzip"), "#!/bin/sh\nprintf '%s\\n' \"$2\" > \""+logPath+"\"\nprintf 'SELECT 1;\\n'\n")
	writeExecutable(t, filepath.Join(binDir, "psql"), "#!/bin/sh\ncat >/dev/null || true\n")

	hostPath := "/System/Volumes/Data/mnt/docker/pentaract-db-backups/" + backupName
	cmd := exec.Command("sh", filepath.Join(scriptsDir, "db-restore.sh"), hostPath)
	cmd.Env = append(
		os.Environ(),
		"BACKUP_DIR="+backupDir,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DATABASE_HOST=db",
		"DATABASE_PORT=5432",
		"DATABASE_USER=pentaract",
		"DATABASE_NAME=pentaract",
		"PGPASSWORD=pentaract",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("db-restore.sh returned error: %v\n%s", err, output)
	}

	loggedPathBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read gunzip log: %v", err)
	}

	loggedPath := strings.TrimSpace(string(loggedPathBytes))
	if loggedPath != resolvedBackupPath {
		t.Fatalf("expected gunzip to read %q, got %q", resolvedBackupPath, loggedPath)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	return wd
}

func writeExecutable(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o755); err != nil {
		t.Fatalf("failed to write executable %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
