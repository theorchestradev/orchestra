//go:build darwin

package sessionhost

/*
#include <errno.h>
#include <fcntl.h>
#include <spawn.h>
#include <stdlib.h>

// Private libSystem calls that Terminal and iTerm2 also use. A disclaimed process answers
// for its own privacy permissions instead of inheriting the app that started it.
extern int responsibility_spawnattrs_setdisclaim(posix_spawnattr_t *attrs, int disclaim);
extern pid_t responsibility_get_pid_responsible_for_pid(pid_t pid);

static int orchestraSpawnHost(pid_t *pid, const char *path, char *const argv[], char *const envp[]) {
	posix_spawnattr_t attrs;
	posix_spawn_file_actions_t actions;
	int err = posix_spawnattr_init(&attrs);
	if (err) return err;
	err = posix_spawn_file_actions_init(&actions);
	if (err) {
		posix_spawnattr_destroy(&attrs);
		return err;
	}
	err = posix_spawnattr_setflags(&attrs, POSIX_SPAWN_SETSID | POSIX_SPAWN_CLOEXEC_DEFAULT);
	if (!err) err = responsibility_spawnattrs_setdisclaim(&attrs, 1);
	if (!err) err = posix_spawn_file_actions_addopen(&actions, 0, "/dev/null", O_RDONLY, 0);
	if (!err) err = posix_spawn_file_actions_addopen(&actions, 1, "/dev/null", O_WRONLY, 0);
	if (!err) err = posix_spawn_file_actions_addopen(&actions, 2, "/dev/null", O_WRONLY, 0);
	// posix_spawn_file_actions_addchdir replaces it only from macOS 26; Orchestra supports 13.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
	if (!err) err = posix_spawn_file_actions_addchdir_np(&actions, "/");
#pragma clang diagnostic pop
	if (!err) err = posix_spawn(pid, path, &actions, &attrs, argv, envp);
	posix_spawn_file_actions_destroy(&actions);
	posix_spawnattr_destroy(&attrs);
	return err;
}
*/
import "C"
import (
	"os"
	"syscall"
	"unsafe"
)

// startHost runs executable as the host in its own session. macOS attributes privacy
// permissions to the process responsible for a process: the host disclaims the app that
// started it, so agents answer to the Orchestra executable the host runs, the same before
// and after the app quits. Otherwise they would answer for themselves once it quits.
func startHost(executable, dir string, env []string) error {
	argv := cStrings([]string{executable, Flag, dir})
	defer freeStrings(argv)
	envp := cStrings(env)
	defer freeStrings(envp)
	var pid C.pid_t
	if err := C.orchestraSpawnHost(&pid, argv[0], &argv[0], &envp[0]); err != 0 {
		return &os.PathError{Op: "spawn", Path: executable, Err: syscall.Errno(err)}
	}
	// Reap the host if it exits while the app is open, for example when another host won.
	if p, err := os.FindProcess(int(pid)); err == nil {
		go func() { _, _ = p.Wait() }()
	}
	return nil
}

// responsiblePID is the process macOS attributes pid's privacy permissions to.
func responsiblePID(pid int) int {
	return int(C.responsibility_get_pid_responsible_for_pid(C.pid_t(pid)))
}

func cStrings(values []string) []*C.char {
	out := make([]*C.char, len(values)+1)
	for i, v := range values {
		out[i] = C.CString(v)
	}
	return out
}
func freeStrings(values []*C.char) {
	for _, v := range values {
		if v != nil {
			C.free(unsafe.Pointer(v))
		}
	}
}
