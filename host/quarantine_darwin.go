//go:build darwin

package main

import "os/exec"

// clearQuarantine снимает с файла метку карантина. macOS ставит её на всё, что создаёт процесс,
// запущенный браузером, — и потом Gatekeeper не даёт запустить неподписанный файл
// («Apple не удалось подтвердить, что файл не содержит вредоносного ПО»). Снимаем её только
// с файлов, которые помощник сам распаковал или скачал и чью контрольную сумму уже проверил.
func clearQuarantine(path string) {
	exec.Command("/usr/bin/xattr", "-d", "com.apple.quarantine", path).Run()
}
