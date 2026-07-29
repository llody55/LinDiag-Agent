package safety

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/google/shlex"
)

// RiskLevel 命令风险等级
type RiskLevel int

const (
	RiskLevelSafe     RiskLevel = iota // 安全：只读查询命令
	RiskLevelLow                       // 低风险：可能有副作用但可控
	RiskLevelMedium                    // 中风险：修改文件或配置，需确认
	RiskLevelHigh                      // 高风险：可能影响系统运行
	RiskLevelCritical                  // 严重：可能导致数据丢失或系统不可用
)

// CommandInfo 命令分析结果
type CommandInfo struct {
	RiskLevel   RiskLevel
	Reason      string
	Explanation string
}

// CommandAnalyzer 命令分析器
type CommandAnalyzer struct{}

// NewCommandAnalyzer 创建命令分析器
func NewCommandAnalyzer() *CommandAnalyzer {
	return &CommandAnalyzer{}
}

// safeCommands 只读安全命令白名单。
// 注意：以下工具不在此列，改由 subCommandRisk 表按子命令分级——
//   - docker / kubectl / systemctl / ip / firewall-cmd / iptables 等多义工具：
//     既有只读子命令 (docker ps / kubectl get / ip addr show)
//     也有变更类子命令 (docker rm / kubectl delete / ip addr del / firewall-cmd --add-port)。
//     整命令名放白名单会让变更类子命令被误判为 Safe 直接执行。
//   - sysctl: 读取为 Safe，写参数(带 = / -w)由 analyzeSysctl 单独识别，故不放白名单。
var safeCommands = []string{
	"uptime", "free", "df", "ps", "top", "dmesg", "ls", "cat", "echo",
	"date", "who", "w", "uname", "hostname", "id", "groups", "netstat",
	"ss", "ifconfig", "ping", "traceroute", "head", "tail", "grep",
	"find", "less", "more", "wc", "du", "which", "whereis", "file", "stat",
	"last", "dig", "nslookup", "journalctl",
	"git", "env", "printenv", "lscpu", "lsof", "vmstat", "iostat", "sar",
	"nproc", "awk", "sed", "sort", "uniq", "tr", "cut", "column",
	"nmap", "tcpdump", "strace", "ltrace", "perf", "bpftool",
	// === Windows 见 safety_windows.go 的 winSafeCommands 补充 ===
}

// subCommandRisk 多义工具的子命令分级表。
// key 为主命令(docker/kubectl/systemctl/ip/iptables 等)，value 为 {只读子命令集合, 变更类子命令集合}。
// - 只读子命令 / 只读 flag → Safe
// - 变更类子命令 / 变更 flag → High（可能影响运行中的服务/容器/pod/网络/防火墙）
// 未在任一集合中列出的子命令 → Medium（走未知子命令分支，避免新出现的高危子命令漏判）。
//
// 为什么这些工具不能整命令名放白名单：
//
//	docker rm / kubectl delete / systemctl stop / ip addr del / firewall-cmd --add-port
//	都会让运维事故瞬间发生，必须按子命令逐一识别。
var subCommandRisk = map[string]struct {
	readOnly map[string]bool
	mutating map[string]bool
}{
	"docker": {
		readOnly: map[string]bool{
			"ps": true, "images": true, "inspect": true, "stats": true,
			"logs": true, "diff": true, "history": true, "info": true,
			"version": true, "search": true,
			"port": true, "top": true, "events": true, "context": true, "manifest": true,
			// docker compose 的只读复合子命令
			"compose ps": true, "compose ls": true, "compose top": true,
			"compose logs": true, "compose config": true, "compose images": true,
			"compose port": true, "compose events": true,
			// docker 命名空间子命令的只读操作
			// 注意：network/volume/container/image/system 不能放单参数 readOnly ——
			// `docker network rm` 会命中 readOnly["network"] 直接返回 Safe 绕过检查！
			// 必须用复合形式 "network ls" / "network inspect" 精确匹配。
			"network ls": true, "network inspect": true,
			"volume ls": true, "volume inspect": true,
			"container ls": true, "container inspect": true, "container logs": true,
			"container stats": true, "container top": true, "container port": true,
			"image ls": true, "image inspect": true, "image history": true,
			"system df": true, "system events": true, "system info": true,
			"secret ls": true, "secret inspect": true,
			"config ls": true, "config inspect": true,
		},
		mutating: map[string]bool{
			"rm": true, "rmi": true, "stop": true, "kill": true,
			"restart": true, "exec": true, "run": true, "create": true,
			"start": true, "pause": true, "unpause": true, "attach": true,
			"wait": true, "export": true, "save": true, "load": true,
			"import": true, "build": true, "commit": true, "tag": true,
			"push": true, "login": true, "logout": true, "prune": true,
			"update": true, "rename": true, "swarm": true, "service": true,
			"node": true, "secret": true, "config": true, "stack": true,
			"deploy": true, "rollback": true, "scale": true,
			// docker compose 的变更复合子命令
			"compose up": true, "compose down": true, "compose stop": true,
			"compose restart": true, "compose start": true, "compose rm": true,
			"compose kill": true, "compose pause": true, "compose unpause": true,
			"compose build": true, "compose pull": true, "compose push": true,
			"compose exec": true, "compose run": true, "compose scale": true,
			// docker 命名空间子命令的变更操作
			"network rm": true, "network create": true, "network disconnect": true,
			"network prune": true, "network connect": true,
			"volume rm": true, "volume create": true, "volume prune": true,
			"container rm": true, "container stop": true, "container kill": true,
			"container start": true, "container restart": true, "container pause": true,
			"container unpause": true, "container exec": true, "container run": true,
			"container create": true, "container update": true, "container rename": true,
			"container attach": true, "container wait": true,
			"image rm": true, "image pull": true, "image push": true, "image build": true,
			"image load": true, "image save": true, "image import": true,
			"image prune": true, "image tag": true,
			"system prune": true,
			"secret rm":    true, "secret create": true,
			"config rm": true, "config create": true,
		},
	},
	"kubectl": {
		readOnly: map[string]bool{
			"get": true, "describe": true, "logs": true, "top": true,
			"explain": true, "version": true, "cluster-info": true,
			"api-resources": true, "api-versions": true, "auth": true,
			"options": true, "config": true, "diff": true,
		},
		mutating: map[string]bool{
			"delete": true, "apply": true, "create": true, "edit": true,
			"patch": true, "replace": true, "run": true, "exec": true,
			"scale": true, "autoscale": true, "expose": true, "set": true,
			"rollout": true, "cordon": true, "uncordon": true, "drain": true,
			"taint": true, "label": true, "annotate": true, "port-forward": true,
			"cp": true, "attach": true, "certificate": true,
		},
	},
	"systemctl": {
		readOnly: map[string]bool{
			"status": true, "is-active": true, "is-enabled": true,
			"is-failed": true, "list-units": true, "list-unit-files": true,
			"show": true, "list-dependencies": true, "list-sockets": true,
			"list-jobs": true, "list-machines": true, "cat": true,
			"help": true, "status-elapsed": true,
		},
		mutating: map[string]bool{
			"start": true, "stop": true, "restart": true, "reload": true,
			"enable": true, "disable": true, "mask": true, "unmask": true,
			"set-default": true, "preset": true, "set-environment": true,
			"unset-environment": true, "import-environment": true,
			"link": true, "daemon-reload": true, "daemon-reexec": true,
			"edit": true, "isolate": true, "kill": true, "clean": true,
		},
	},

	// === iproute2：网络配置 ===
	// `ip` 命令: addr/route/link/neigh/maddr/rule/tunnel 的 show/list/get → 只读；
	// add/del/set/change/replace/flush → 变更（可瞬间改主机网络、断网、路由黑洞）。
	// 复合子命令形式: `ip addr show`, `ip route add`, `ip link set dev eth0 up`
	"ip": {
		readOnly: map[string]bool{
			"addr show": true, "addr list": true, "address show": true, "address list": true,
			"route show": true, "route list": true, "route get": true, "route lookup": true,
			"link show": true, "link list": true,
			"neigh show": true, "neigh list": true, "neighbour show": true,
			"maddr show": true, "maddress show": true,
			"rule show": true, "rule list": true,
			"tunnel show": true, "tunnel list": true,
			"netns list":      true,
			"xfrm state show": true, "xfrm state list": true, "xfrm state get": true,
			"xfrm policy show": true, "xfrm policy list": true,
			"help": true, "version": true,
			// 一些仅打印的简写: `ip a` (addr), `ip r` (route), `ip l` (link), `ip n` (neigh)
			"a": true, "r": true, "l": true, "n": true, "m": true,
		},
		mutating: map[string]bool{
			"addr add": true, "addr del": true, "addr change": true, "addr replace": true,
			"address add": true, "address del": true, "address change": true, "address replace": true,
			"addr flush": true, "address flush": true,
			"route add": true, "route del": true, "route change": true, "route replace": true,
			"route flush": true,
			"link set":    true, "link add": true, "link del": true, "link change": true,
			"link replace": true,
			"neigh add":    true, "neigh del": true, "neigh change": true, "neigh replace": true,
			"neigh flush": true, "neighbour add": true, "neighbour del": true,
			"maddr add": true, "maddr del": true, "maddress add": true, "maddress del": true,
			"rule add": true, "rule del": true, "rule flush": true,
			"tunnel add": true, "tunnel del": true, "tunnel change": true, "tunnel replace": true,
			"netns add": true, "netns del": true, "netns set": true, "netns exec": true,
			// IPsec 策略和状态变更
			"xfrm state add": true, "xfrm state del": true, "xfrm state update": true, "xfrm state alloc": true,
			"xfrm policy add": true, "xfrm policy del": true, "xfrm policy update": true,
			"xfrm policy set": true, "xfrm policy flush": true, "xfrm state flush": true,
			// 简写形式: `ip a add`, `ip r del`, `ip l set` (萎缩形式也作为变更类)
			"a add": true, "a del": true, "a change": true, "a replace": true, "a flush": true,
			"r add": true, "r del": true, "r change": true, "r replace": true, "r flush": true,
			"l set": true, "l add": true, "l del": true, "l change": true, "l replace": true,
			"n add": true, "n del": true, "n change": true, "n replace": true, "n flush": true,
		},
	},

	// === 防火墙工具 ===
	// iptables: -L/-S/-nL 等列表为只读；-A/-D/-I/-F/-X/-Z/-P/-N 为变更
	// iptables 主要靠 flag 而非子命令区分读写，用空串""作为 key 在子命令位置匹配 flag 前缀
	"iptables": {
		readOnly: map[string]bool{
			// 子命令位置是 flag 形式
			"-L": true, "--list": true,
			"-S": true, "--list-rules": true,
			"-n -L":     true, // 常见组合: iptables -nL 也归只读
			"-v -L":     true,
			"--version": true, "-h": true, "--help": true,
		},
		mutating: map[string]bool{
			"-A": true, "--append": true,
			"-D": true, "--delete": true,
			"-I": true, "--insert": true,
			"-R": true, "--replace": true,
			"-F": true, "--flush": true,
			"-Z": true, "--zero": true,
			"-N": true, "--new-chain": true,
			"-X": true, "--delete-chain": true,
			"-P": true, "--policy": true,
			"-E": true, "--rename-chain": true,
			"-C": true, "--check": true, // check 本身只读，但常被用做存在性校验后配合 -A，保守归只读以下级别
		},
	},
	"ip6tables": {
		readOnly: map[string]bool{
			"-L": true, "--list": true, "-S": true, "--list-rules": true,
			"-n -L": true, "-v -L": true,
			"--version": true, "-h": true, "--help": true,
		},
		mutating: map[string]bool{
			"-A": true, "--append": true, "-D": true, "--delete": true,
			"-I": true, "--insert": true, "-R": true, "--replace": true,
			"-F": true, "--flush": true, "-Z": true, "--zero": true,
			"-N": true, "--new-chain": true, "-X": true, "--delete-chain": true,
			"-P": true, "--policy": true, "-E": true, "--rename-chain": true,
		},
	},
	// firewall-cmd (firewalld): --list-* / --get-* / --info-* 为只读；--add-* / --remove-* / --change-* / --reload / --permanent 为变更
	"firewall-cmd": {
		readOnly: map[string]bool{
			"--list-all": true, "--list-all-zones": true,
			"--list-interfaces": true, "--list-ports": true, "--list-services": true,
			"--list-rich-rules": true, "--list-source": true, "--list-icmp-blocks": true,
			"--list-forward-ports": true, "--list-masquerade": true,
			"--get-default-zone": true, "--get-active-zones": true,
			"--get-zones": true, "--get-services": true, "--get-icmptypes": true,
			"--info-zone": true, "--info-service": true, "--info-icmp-type": true,
			"--info-ipset": true, "--info-zone=": true,
			"--version": true, "--help": true, "--state": true, "--reload-check": true,
		},
		mutating: map[string]bool{
			"--add-port": true, "--add-service": true, "--add-interface": true,
			"--add-source": true, "--add-rich-rule": true, "--add-masquerade": true,
			"--add-forward-port": true, "--add-icmp-block": true,
			"--remove-port": true, "--remove-service": true, "--remove-interface": true,
			"--remove-source": true, "--remove-rich-rule": true, "--remove-masquerade": true,
			"--remove-forward-port": true, "--remove-icmp-block": true,
			"--change-interface": true, "--change-source": true, "--change-zone": true,
			"--add-zone": true, "--remove-zone": true, "--set-default-zone": true,
			"--reload": true, "--complete-reload": true, "--runtime-to-permanent": true,
			"--permanent": true, "--new-service": true, "--new-zone": true, "--new-ipset": true,
			"--delete-service": true, "--delete-zone": true, "--delete-ipset": true,
			"--load-service-defaults": true, "--load-zone-defaults": true,
			"--add-protocol": true, "--remove-protocol": true,
			"--add-source-port": true, "--remove-source-port": true,
			"--panic-on": true, "--panic-off": true,
		},
	},
	// ufw: status/verbose/show 为只读；allow/deny/enable/disable/default/delete/reset 为变更
	"ufw": {
		readOnly: map[string]bool{
			"status": true, "--version": true, "version": true, "help": true,
		},
		mutating: map[string]bool{
			"allow": true, "deny": true, "reject": true, "limit": true,
			"enable": true, "disable": true, "default": true, "delete": true,
			"reset": true, "reload": true, "app": true,
		},
	},
	// nft (nftables): list 为只读；add/insert/delete/flush/replace/change 为变更
	"nft": {
		readOnly: map[string]bool{
			"list": true, "--help": true, "--version": true, "-h": true,
		},
		mutating: map[string]bool{
			"add": true, "insert": true, "delete": true, "flush": true,
			"replace": true, "change": true, "create": true,
		},
	},
	// ipset: list/save 为只读；add/del/create/destroy/swap/flush/rename/store/restore 为变更
	"ipset": {
		readOnly: map[string]bool{
			"list": true, "save": true, "help": true, "version": true,
		},
		mutating: map[string]bool{
			"add": true, "del": true, "create": true, "destroy": true,
			"swap": true, "flush": true, "rename": true, "test": true,
			"restore": true, "store": true,
		},
	},

	// === systemd 变种工具（hostnamectl/timedatectl/localectl） ===
	// show / status 为只读；set-* / set-*-* 为变更
	"hostnamectl": {
		readOnly: map[string]bool{
			"status": true, "--help": true, "--version": true,
		},
		mutating: map[string]bool{
			"set-hostname": true, "set-icon-name": true, "set-chassis": true,
			"set-deployment": true, "set-location": true,
		},
	},
	"timedatectl": {
		readOnly: map[string]bool{
			"status": true, "show": true, "timesync-status": true,
			"show-timesync": true, "--help": true, "--version": true,
		},
		mutating: map[string]bool{
			"set-time": true, "set-timezone": true, "set-local-rtc": true,
			"set-ntp": true, "set-default-timezone": true,
		},
	},
	"localectl": {
		readOnly: map[string]bool{
			"status": true, "list-locales": true, "list-keymaps": true,
			"--help": true, "--version": true,
		},
		mutating: map[string]bool{
			"set-locale": true, "set-keymap": true, "set-x11-keymap": true,
		},
	},
	"loginctl": {
		readOnly: map[string]bool{
			"list-sessions": true, "list-users": true, "show-session": true,
			"show-user": true, "session-status": true, "user-status": true,
		},
		mutating: map[string]bool{
			"lock-session": true, "unlock-session": true, "lock-sessions": true,
			"unlock-sessions": true, "terminate-session": true, "terminate-user": true,
			"kill-session": true, "kill-user": true, "enable-linger": true,
			"disable-linger": true, "attach": true,
		},
	},

	// crontab: -l 为只读；-e/-r/-i 为变更
	"crontab": {
		readOnly: map[string]bool{
			"-l": true, "--list": true, "-V": true, "--version": true,
		},
		mutating: map[string]bool{
			"-e": true, "--edit": true, "-r": true, "--remove": true,
			"-i": true, "--prompt": true, "-s": true,
		},
	},

	// === 容器/编排生态 ===
	// docker-compose v1（独立二进制，与 docker compose v2 行为相同）
	"docker-compose": {
		readOnly: map[string]bool{
			"ps": true, "ls": true, "top": true, "logs": true,
			"config": true, "images": true, "port": true, "events": true,
			"version": true, "help": true,
		},
		mutating: map[string]bool{
			"up": true, "down": true, "stop": true, "restart": true,
			"start": true, "rm": true, "kill": true, "pause": true,
			"unpause": true, "build": true, "pull": true, "push": true,
			"exec": true, "run": true, "scale": true, "create": true,
		},
	},
	// helm: list/get/search/show/values/status/test/template/lint 为只读；
	// install/uninstall/upgrade/rollback/create/delete/package/push/pull 为变更
	"helm": {
		readOnly: map[string]bool{
			"list": true, "ls": true, "get": true, "search": true, "show": true,
			"values": true, "status": true, "test": true, "version": true,
			"history": true, "dependency": true, "env": true, "help": true,
			"repo list": true,
			// template 只生成 YAML 不变更集群；lint 只检查 chart 语法不写入
			"template": true, "lint": true,
		},
		mutating: map[string]bool{
			"install": true, "uninstall": true, "upgrade": true, "rollback": true,
			"create": true, "delete": true, "package": true, "push": true,
			"pull": true, "registry": true,
			"repo add": true, "repo remove": true, "repo update": true,
			"repo index": true, "plugin install": true, "plugin uninstall": true,
			"enc": true, "dep up": true, "dependency update": true,
		},
	},
	// kubeadm: config print/init/alpha 为只读；reset/init/join/upgrade/token apply/cert renew 为变更（影响集群）
	"kubeadm": {
		readOnly: map[string]bool{
			"config print": true, "config images list": true,
			"token list": true, "version": true, "alpha certs check-expiration": true,
		},
		mutating: map[string]bool{
			"init": true, "join": true, "reset": true, "upgrade": true,
			"token create": true, "token delete": true, "token generate": true,
			"certs renew": true, "certs re-new": true,
			"kubeconfig": true, "alpha certs renew": true, "alpha kubeconfig": true,
			"node-phase": true, "control-plane": true,
		},
	},
	// etcdctl: get/endpoint health/status/member list/snapshot status 为只读；
	// put/del/member add/remove/upgrade/snapshot save/restore/move-leader/compact/defrag 为变更
	"etcdctl": {
		readOnly: map[string]bool{
			"get": true, "endpoint health": true, "endpoint status": true,
			"member list": true, "snapshot status": true, "alarm list": true,
			"role list": true, "user list": true, "auth get": true,
			"--help": true,
		},
		mutating: map[string]bool{
			"put": true, "del": true, "member add": true, "member remove": true,
			"member update": true, "member promote": true, "snapshot save": true,
			"snapshot restore": true, "move-leader": true, "compact": true,
			"defrag": true, "alarm disarm": true, "alarm set": true,
			"role add": true, "role delete": true, "role grant": true, "role revoke": true,
			"user add": true, "user delete": true, "user grant": true, "user revoke": true,
			"auth enable": true, "auth disable": true, "lock": true, "elect": true,
		},
	},
	// crictl (kubernetes CRI CLI): info/logs/stats/inspect/ps/version/port 为只读；
	// exec/create/start/stop/rm/rmi/pull/run/update 为变更
	"crictl": {
		readOnly: map[string]bool{
			"info": true, "logs": true, "stats": true, "inspect": true,
			"ps": true, "pods": true, "images": true, "version": true, "port": true,
		},
		mutating: map[string]bool{
			"exec": true, "create": true, "start": true, "stop": true,
			"rm": true, "rmi": true, "pull": true, "run": true, "update": true,
			"checkpoint": true, "remove": true,
		},
	},
	// ctr (containerd CLI): 子命令空间分层（images/containers/tasks/run）
	"ctr": {
		readOnly: map[string]bool{
			"images ls": true, "images list": true, "images check": true,
			"containers ls": true, "containers list": true,
			"tasks ls": true, "tasks list": true,
			"info": true, "version": true, "plugins ls": true,
			"content ls":    true,
			"namespaces ls": true, "leases ls": true,
		},
		mutating: map[string]bool{
			"images pull": true, "images push": true, "images rm": true,
			"images tag": true, "images import": true, "images export": true,
			"containers create": true, "containers rm": true, "containers start": true,
			"tasks start": true, "tasks kill": true, "tasks delete": true,
			"run": true, "content ingest": true, "content delete": true,
			"content fetch":     true, // fetch 会下载内容写入本地存储，不是纯只读
			"namespaces create": true, "namespaces rm": true,
			"leases create": true, "leases delete": true,
		},
	},
	// nerdctl (containerd user CLI): 类 docker 行为
	"nerdctl": {
		readOnly: map[string]bool{
			"ps": true, "images": true, "inspect": true, "logs": true,
			"stats": true, "info": true, "version": true, "history": true,
			"compose ps": true, "compose logs": true, "compose config": true,
			"compose images": true,
		},
		mutating: map[string]bool{
			"rm": true, "rmi": true, "stop": true, "kill": true, "restart": true,
			"exec": true, "run": true, "create": true, "start": true, "pause": true,
			"unpause": true, "build": true, "commit": true, "tag": true, "push": true,
			"login": true, "logout": true, "prune": true,
			"pull": true, "save": true, "load": true, "import": true, "export": true,
			"compose up": true, "compose down": true, "compose stop": true,
			"compose restart": true, "compose start": true, "compose rm": true,
			"compose kill": true, "compose pause": true, "compose unpause": true,
			"compose build": true, "compose pull": true, "compose push": true,
			"compose exec": true, "compose run": true,
		},
	},
	// podman: 类 docker 行为
	"podman": {
		readOnly: map[string]bool{
			"ps": true, "images": true, "inspect": true, "logs": true,
			"stats": true, "info": true, "version": true, "history": true,
			"diff": true, "port": true, "top": true, "events": true,
			"search": true, "machine list": true, "machine inspect": true,
			"compose ps": true, "compose logs": true, "compose config": true,
		},
		mutating: map[string]bool{
			"rm": true, "rmi": true, "stop": true, "kill": true, "restart": true,
			"exec": true, "run": true, "create": true, "start": true, "pause": true,
			"unpause": true, "build": true, "commit": true, "tag": true, "push": true,
			"pull": true, "login": true, "logout": true, "prune": true,
			"save": true, "load": true, "import": true, "export": true,
			"compose up": true, "compose down": true, "compose stop": true,
			"compose restart": true, "compose start": true, "compose rm": true,
			"compose kill": true, "compose build": true, "compose pull": true,
			"compose push": true, "compose exec": true, "compose run": true,
			"machine init": true, "machine rm": true, "machine start": true,
			"machine stop": true, "machine restart": true,
			"volume create": true, "volume rm": true, "volume prune": true,
			"network create": true, "network rm": true, "network prune": true,
			"secret create": true, "secret rm": true,
		},
	},
}

// mutatingSubCommandRiskLevel 多义工具变更类子命令的默认风险等级。
// docker rm / kubectl delete / systemctl stop 都可能影响线上服务，High。
const mutatingSubCommandRiskLevel = RiskLevelHigh

// mediumRiskCommands 有副作用但通常可控的命令
// 注意：sysctl 不在此列 —— 它的读/写在 step 7b 由 analyzeSysctl 单独识别。
// 这里列出的是整命令名都具备副作用的工具（无法只读取）。
var mediumRiskCommands = []string{
	"curl", "wget", "ssh", "scp", "rsync", "ftp", "nc", "telnet",
	// 网络副作用工具：这些命令的"读"和"写"不可静态区分，整命令名一视 Medium
	"tc",      // 流量控制：tc qdisc show 看似只读但 tc qdisc add 改 QoS
	"ethtool", // -S 看统计为只读，-s 改网卡设置为变更；保守 Medium
	"route",   // net-tools 路由：route -n 看路由表为只读，route add/del 为变更；保守 Medium
	"arp",     // arp -n 看为只读，arp -d/-s 为变更；保守 Medium
	"tee",     // tee 写文件：常被用以绕过 sudo 不能重定向的限制，Medium 询问
	"logger",  // 向 syslog 写日志，副作用
	"wall",    // 广播消息给所有终端，打扰用户
	"write",   // 向其他用户终端写消息
	"at",      // atq 看为只读，at/atrM 为变更；保守 Medium
	"batch",   // 类 at
	"tar",     // tar -x 提取会写文件，tar -c 打包正常；保守 Medium
	"zip",     // 写压缩文件
	"unzip",   // 提取会写文件
	"gzip",    // 压缩会替换原文件
	"gunzip",  // 解压会替换原文件
	"bzip2",   // 压缩会替换原文件
	"bunzip2", // 解压会替换原文件
	"xz",      // 压缩会替换原文件
	"unxz",    // 解压会替换原文件
	"zstd",    // 压缩/解压
}

// dangerousCommands 高危命令，默认 Critical
// 这些命令一旦执行错误可能：删除数据 / 关机 / 改账号 / 改磁盘 / 改内核模块。
// 主命令匹配规则：cmdName == dangerName || strings.HasPrefix(cmdName, dangerName+".")
//
//	例如 mkfs.ext4 命中 "mkfs"
var dangerousCommands = map[string]string{
	// === 文件/系统破坏 ===
	"rm":       "删除文件或目录，错误使用可能导致数据丢失",
	"mkfs":     "格式化磁盘，会清除所有数据",
	"dd":       "数据复制工具，错误使用可能覆盖重要数据",
	"reboot":   "重启系统，当前工作可能丢失",
	"poweroff": "关闭系统电源，所有未保存的工作将丢失",
	"shutdown": "关闭系统，所有未保存的工作将丢失",
	"halt":     "停止系统，所有未保存的工作将丢失",
	"umount":   "卸载文件系统，可能导致数据丢失",
	"mount":    "挂载文件系统，错误挂载可能导致数据损坏",
	"fsck":     "检查和修复文件系统，可能导致数据丢失",
	// init / telinit 改运行级别，影响系统状态
	"init":    "切换系统运行级别，可能影响所有服务",
	"telinit": "切换系统运行级别，可能影响所有服务",
	// === 账号管理（改账号 = 改权限边界） ===
	"useradd":  "创建用户账号，影响系统访问权限",
	"userdel":  "删除用户账号，用户将无法登录且数据可能被删除",
	"usermod":  "修改用户账号属性，可能改变权限边界",
	"groupadd": "创建用户组，影响系统访问权限",
	"groupdel": "删除用户组，影响系统访问权限",
	"groupmod": "修改用户组属性，可能改变权限边界",
	"passwd":   "修改用户密码，用户登录凭据将被改变",
	"chage":    "修改用户密码过期策略，影响登录",
	"gpasswd":  "修改组密码或组成员，影响权限",
	"chpasswd": "批量修改用户密码，影响登录凭据",
	"visudo":   "编辑 sudoers 文件，影响提权规则（虽防语法错误但仍改权限）",
	"newusers": "批量创建/修改用户，影响系统访问权限",
	// === 存储管理（改磁盘结构/逻辑卷） ===
	"swapon":     "启用 swap，改变系统内存行为",
	"swapoff":    "禁用 swap，可能导致内存压力骤增",
	"mkswap":     "格式化 swap 区域，覆盖目标设备数据",
	"lvcreate":   "创建 LVM 逻辑卷，改变存储结构",
	"lvremove":   "删除 LVM 逻辑卷，数据将丢失",
	"lvextend":   "扩展 LVM 逻辑卷，操作不当可能损坏文件系统",
	"lvreduce":   "缩小 LVM 逻辑卷，可能导致数据丢失",
	"lvresize":   "调整 LVM 逻辑卷大小，操作不当可能损坏文件系统",
	"lvchange":   "改变 LVM 逻辑卷属性，影响可用性",
	"lvrename":   "重命名 LVM 逻辑卷，影响挂载/配置",
	"pvcreate":   "初始化 LVM 物理卷，覆盖目标设备分区表",
	"pvremove":   "删除 LVM 物理卷元数据",
	"vgcreate":   "创建 LVM 卷组，改变存储结构",
	"vgremove":   "删除 LVM 卷组，数据将丢失",
	"vgextend":   "扩展 LVM 卷组，改变存储结构",
	"vgreduce":   "缩小 LVM 卷组，可能导致数据丢失",
	"vgexport":   "导出 LVM 卷组，使其不被主机识别，影响可用性",
	"vgimport":   "导入 LVM 卷组，改变主机存储视图",
	"vgscan":     "扫描 LVM 卷组，重建缓存，可能影响设备映射",
	"mdadm":      "管理软件 RAID，错误操作可能导致阵列损坏和数据丢失",
	"fdisk":      "修改磁盘分区表，错误操作可能导致数据丢失",
	"parted":     "修改磁盘分区表，错误操作可能导致数据丢失",
	"sfdisk":     "修改磁盘分区表，错误操作可能导致数据丢失",
	"cfdisk":     "修改磁盘分区表，错误操作可能导致数据丢失",
	"gdisk":      "修改 GPT 分区表，错误操作可能导致数据丢失",
	"sgdisk":     "修改 GPT 分区表（非交互），错误操作可能导致数据丢失",
	"resize2fs":  "调整 ext2/3/4 文件系统大小，操作不当可能损坏文件系统",
	"xfs_growfs": "扩展 XFS 文件系统，操作不当可能损坏文件系统",
	"xfs_repair": "修复 XFS 文件系统，可能导致数据丢失",
	"btrfs":      "Btrfs 子卷/设备管理，错误操作可能导致数据丢失",
	"dmsetup":    "Device Mapper 管理，错误操作可能导致块设备映射失效",
	"losetup":    "设置循环设备，改变块设备映射",
	"blkdiscard": "丢弃块设备扇区（TRIM），数据将被永久清除",
	"wipefs":     "擦除文件系统签名，设备将无法挂载",
	// === 内核模块 ===
	"modprobe": "加载/卸载内核模块，可能影响内核行为和设备驱动",
	"rmmod":    "卸载内核模块，可能影响内核行为和设备驱动",
	"insmod":   "加载内核模块，可能影响内核行为和设备驱动",
	// === 内核/系统行为直接变更 ===
	"setcap":  "设置文件 capabilities，改变程序权限边界",
	"setfacl": "设置文件 ACL，改变访问权限",
	"chacl":   "更改文件 ACL，改变访问权限",
	// === Windows 见 safety_windows.go 的 winDangerousCommands 补充 ===
}

// mediumRiskCommandsMap 方便查找
var mediumRiskCommandsMap map[string]bool

func init() {
	mediumRiskCommandsMap = make(map[string]bool, len(mediumRiskCommands))
	for _, c := range mediumRiskCommands {
		mediumRiskCommandsMap[c] = true
	}
}

// 危险模式正则
var riskyPatterns = []struct {
	pattern     *regexp.Regexp
	description string
}{
	{regexp.MustCompile(`rm\s+-[rf]+\s+/(\s|$)`), "递归删除根目录"},
	{regexp.MustCompile(`rm\s+-[rf]+\s+\.{2}`), "删除上级目录"},
	{regexp.MustCompile(`mkfs\s+`), "格式化磁盘"},
	{regexp.MustCompile(`dd\s+if=\S+\s+of=/dev/`), "向设备写入数据"},
	{regexp.MustCompile(`chmod\s+777\s+/(?:\S*)?(?:\s|$)`), "全局可写关键目录"},
	{regexp.MustCompile(`chown\s+\S+:\S+\s+/(?:\S*)?(?:\s|$)`), "更改关键目录所有权"},
	{regexp.MustCompile(`:\(\)\s*\{`), "fork bomb"},
	{regexp.MustCompile(`>\s*/dev/sd`), "直接写入磁盘设备"},
	// === Windows 危险模式 ===
	// Remove-Item -Recurse -Force C:\ 递归强制删除（PowerShell 删根）
	{regexp.MustCompile(`(?i)Remove-Item\s+.*-Recurse.*-Force.*:\\`), "递归强制删除根盘符"},
	{regexp.MustCompile(`(?i)Remove-Item\s+.*-Force.*-Recurse.*:\\`), "递归强制删除根盘符"},
	// format C: 格式化盘符
	{regexp.MustCompile(`(?i)format\s+[A-Z]:`), "格式化磁盘卷"},
	// diskpart /s 可改分区表（危险但非正则可精确匹配，保守拦截裸 diskpart 调用）
	{regexp.MustCompile(`(?i)\bdiskpart\b`), "磁盘分区管理工具，可改分区表"},
	// Shutdown/Restart-Computer 关机重启
	{regexp.MustCompile(`(?i)(Stop-Computer|Restart-Computer|shutdown\s+/s|shutdown\s+/r)`), "关机或重启系统"},
	// Clear-Disk / Clear-RecycleBin 危险
	{regexp.MustCompile(`(?i)Clear-Disk`), "清除磁盘数据"},
}

// 命令注入向量：这些命令可以执行任意子命令，需递归分析其参数
var commandExecutors = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ash": true,
	"exec": true, "eval": true, "source": true, ".": true,
	"xargs": true, "su": true, "sudo": true,
	// Windows: PowerShell / cmd / pwsh 启动器
	"powershell": true, "pwsh": true, "cmd": true,
	// Windows cmd 内置命令注入：cmd /c 和 PowerShell -Command 可执行任意子命令，
	// 由 analyzeCommandExecutor 的 Windows 扩展识别 -Command / /c 参数。
}

// commandDescriptions 命令描述
var commandDescriptions = map[string]string{
	"uptime":     "查看系统运行时间和负载",
	"free":       "查看内存使用情况",
	"df":         "查看磁盘使用情况",
	"ps":         "查看进程状态",
	"top":        "实时查看系统资源使用",
	"dmesg":      "查看内核日志",
	"ls":         "列出目录内容",
	"cat":        "查看文件内容",
	"echo":       "输出文本",
	"date":       "显示日期时间",
	"who":        "查看当前登录用户",
	"w":          "查看系统负载和用户",
	"uname":      "查看系统信息",
	"hostname":   "查看主机名",
	"id":         "查看用户ID信息",
	"groups":     "查看用户组",
	"netstat":    "查看网络连接状态",
	"ss":         "查看网络套接字",
	"ifconfig":   "查看网络接口配置",
	"ip":         "网络配置工具",
	"ping":       "测试网络连通性",
	"traceroute": "追踪路由",
	"curl":       "网络请求工具",
	"wget":       "下载文件",
	"head":       "查看文件头部",
	"tail":       "查看文件尾部",
	"grep":       "文本搜索",
	"find":       "查找文件",
	"less":       "分页查看文件",
	"more":       "分页查看文件",
	"wc":         "统计文件行数/字数",
	"du":         "查看目录大小",
	"which":      "查找命令位置",
	"whereis":    "查找命令位置",
	"file":       "识别文件类型",
	"stat":       "查看文件状态",
	"last":       "查看登录历史",
	"dig":        "DNS查询",
	"nslookup":   "DNS查询",
	"systemctl":  "系统服务管理",
	"journalctl": "查看系统日志",
	"kubectl":    "Kubernetes命令行工具",
	"docker":     "Docker命令行工具",
	"git":        "版本控制工具",
	"env":        "查看环境变量",
	"printenv":   "查看环境变量",
	"lscpu":      "查看CPU信息",
	"lsof":       "查看打开的文件",
	"vmstat":     "查看虚拟内存统计",
	"iostat":     "查看IO统计",
	"awk":        "文本处理",
	"sed":        "文本替换",
	"sort":       "排序",
	"uniq":       "去重",
	"tcpdump":    "抓包工具",
	"nmap":       "网络扫描",
}

// safeCommandSet 用于 O(1) 查找的集合，与 safeCommands 保持同步
var safeCommandSet map[string]bool

// safeMu 保护 safeCommands 和 safeCommandSet 的并发读写
var safeMu sync.RWMutex

func init() {
	rebuildSafeCommandSet()
}

// rebuildSafeCommandSet 根据 safeCommands 切片重建查找集合（自动去重）
func rebuildSafeCommandSet() {
	safeCommandSet = make(map[string]bool, len(safeCommands))
	for _, c := range safeCommands {
		safeCommandSet[c] = true
	}
	// 如果原切片有重复，重建去重后的切片
	if len(safeCommandSet) != len(safeCommands) {
		safeCommands = safeCommands[:0]
		for c := range safeCommandSet {
			safeCommands = append(safeCommands, c)
		}
	}
}

// LoadWhitelist 从文件加载安全命令白名单。
// 用户白名单是硬编码列表的扩展而非替换：合并去重。
// 这样用户的 whitelist.txt 即使只列了部分命令，也不会丢失内置安全命令。
func LoadWhitelist(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	safeMu.Lock()
	defer safeMu.Unlock()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// 仅添加尚未存在的命令，避免重复
		if !safeCommandSet[line] {
			safeCommands = append(safeCommands, line)
			safeCommandSet[line] = true
		}
	}

	return scanner.Err()
}

// GetSafeCommands 获取安全命令列表
func GetSafeCommands() []string {
	safeMu.RLock()
	defer safeMu.RUnlock()
	return safeCommands
}

// isSafeCommand 判断命令名是否在安全白名单中
func isSafeCommand(name string) bool {
	safeMu.RLock()
	defer safeMu.RUnlock()
	return safeCommandSet[name]
}

// AnalyzeCommand 分析命令的风险等级。
// 使用 shlex 正确解析 shell 命令，递归分析管道和 && 链中的子命令。
func (a *CommandAnalyzer) AnalyzeCommand(cmd string) *CommandInfo {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return &CommandInfo{RiskLevel: RiskLevelSafe, Reason: "空命令", Explanation: "这是一个空命令"}
	}

	// 先检查危险正则模式
	for _, rule := range riskyPatterns {
		if rule.pattern.MatchString(cmd) {
			return &CommandInfo{
				RiskLevel:   RiskLevelCritical,
				Reason:      "命令匹配危险模式: " + rule.description,
				Explanation: "该命令执行后可能导致系统严重损坏，请谨慎操作",
			}
		}
	}

	// 分析整个命令链（处理管道和逻辑操作符）
	return a.analyzeCommandChain(cmd)
}

// analyzeCommandChain 分析可能包含管道、&&、; 的命令链
func (a *CommandAnalyzer) analyzeCommandChain(cmd string) *CommandInfo {
	subCmds := splitCommandChain(cmd)
	if len(subCmds) == 1 {
		return a.analyzeSingleCommand(subCmds[0])
	}

	// 管道右段递归分析：当管道右侧是命令执行器（bash/sh/zsh 等）时，
	// 管道左侧的内容作为子命令递归分析风险，
	// 避免 echo "rm -rf /" | bash 被误判为 Low（echo Safe + bash 无 -c High 但原因不精准）。
	if len(subCmds) == 2 {
		rightParts, err := shlex.Split(subCmds[1])
		if err != nil {
			rightParts = strings.Fields(subCmds[1])
		}
		if len(rightParts) > 0 && commandExecutors[rightParts[0]] {
			// 管道右侧是命令执行器，递归分析左侧内容
			leftInfo := a.AnalyzeCommand(subCmds[0])
			// 管道执行器最低 Medium（绕过直接调用路径）
			pipeRisk := leftInfo.RiskLevel
			if pipeRisk < RiskLevelMedium {
				pipeRisk = RiskLevelMedium
			}
			pipeInfo := &CommandInfo{
				RiskLevel:   pipeRisk,
				Reason:      fmt.Sprintf("管道向 %s 传入: %s", rightParts[0], leftInfo.Reason),
				Explanation: leftInfo.Explanation,
			}
			// 同时分析右侧自身风险（如 xargs rm → Critical），
			// 取两者中更高的风险等级
			rightInfo := a.analyzeSingleCommand(subCmds[1])
			if rightInfo.RiskLevel > pipeInfo.RiskLevel {
				return rightInfo
			}
			return pipeInfo
		}
	}

	maxRisk := RiskLevelSafe
	var reason, explanation string
	for _, sub := range subCmds {
		info := a.analyzeSingleCommand(sub)
		if info.RiskLevel > maxRisk {
			maxRisk = info.RiskLevel
			reason = info.Reason
			explanation = info.Explanation
		}
	}
	if maxRisk == RiskLevelSafe {
		return &CommandInfo{
			RiskLevel:   RiskLevelLow,
			Reason:      "命令包含管道或多命令操作",
			Explanation: "命令链中所有子命令均为安全命令",
		}
	}
	return &CommandInfo{
		RiskLevel:   maxRisk,
		Reason:      "命令链中包含高风险操作: " + reason,
		Explanation: explanation,
	}
}

// splitCommandChain 将命令按管道和逻辑操作符分割为子命令。
// 使用 rune 状态机扫描，仅在引号外部识别 |、&&、||、; 操作符，
// 避免引号内的这些字符被错误分割（如 sh -c "rm -rf / | nc evil"）。
func splitCommandChain(cmd string) []string {
	var parts []string
	var buf []rune
	quote := rune(0) // 0=无引号, '\''=单引号, '"'=双引号

	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		// 引号状态切换
		if quote == 0 {
			if r == '\'' || r == '"' {
				quote = r
				buf = append(buf, r)
				continue
			}
		} else {
			// 在引号内：只有匹配的引号才能关闭
			if r == quote {
				quote = 0
			}
			buf = append(buf, r)
			continue
		}

		// 引号外：检查多字符操作符 && 和 ||
		if i+1 < len(runes) {
			two := string(runes[i : i+2])
			if two == "&&" || two == "||" {
				part := strings.TrimSpace(string(buf))
				if part != "" {
					parts = append(parts, part)
				}
				buf = nil
				i++ // skip second char
				continue
			}
		}

		// 引号外：检查单字符操作符 | 和 ;
		if r == '|' || r == ';' {
			part := strings.TrimSpace(string(buf))
			if part != "" {
				parts = append(parts, part)
			}
			buf = nil
			continue
		}

		buf = append(buf, r)
	}

	// 尾部剩余
	part := strings.TrimSpace(string(buf))
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}

// analyzeSingleCommand 分析单条命令（不含管道和逻辑操作符）
func (a *CommandAnalyzer) analyzeSingleCommand(cmd string) *CommandInfo {
	// 使用 shlex 正确解析（处理引号）
	parts, err := shlex.Split(cmd)
	if err != nil {
		// shlex 解析失败，降级为简单分割
		parts = strings.Fields(cmd)
	}
	if len(parts) == 0 {
		return &CommandInfo{RiskLevel: RiskLevelSafe, Reason: "空命令", Explanation: "这是一个空命令"}
	}

	cmdName := parts[0]

	// 1. 检查命令注入向量（sh -c, bash -c, xargs, sudo 等）— 递归分析
	if commandExecutors[cmdName] {
		return a.analyzeCommandExecutor(cmdName, parts, cmd)
	}

	// 2. 检查 find -exec 中的危险操作
	if cmdName == "find" && strings.Contains(cmd, "-exec") {
		if info := a.analyzeFindExec(cmd); info != nil {
			return info
		}
	}

	// 3. 检查重定向（写入文件）
	if hasWriteRedirect(cmd) {
		// 如果命令本身是安全的，重定向写入为 Medium
		if isSafeCommand(cmdName) {
			return &CommandInfo{
				RiskLevel:   RiskLevelMedium,
				Reason:      "命令包含输出重定向，可能创建或覆盖文件",
				Explanation: getDesc(cmdName) + "，输出重定向到文件",
			}
		}
	}

	// 4. 检查危险命令（含前缀匹配，如 mkfs.ext4 匹配 mkfs）
	for dangerName, desc := range dangerousCommands {
		if cmdName == dangerName || strings.HasPrefix(cmdName, dangerName+".") {
			// sed -i 是原地修改，Critical
			if dangerName == "sed" && containsFlag(parts, "-i") {
				return &CommandInfo{
					RiskLevel:   RiskLevelCritical,
					Reason:      "sed -i 会直接修改文件内容",
					Explanation: "sed -i 命令会原地修改文件，可能导致文件损坏",
				}
			}
			return &CommandInfo{
				RiskLevel:   RiskLevelCritical,
				Reason:      "命令包含危险的 " + dangerName + " 操作",
				Explanation: desc,
			}
		}
	}

	// 5. 检查 sed -i（sed 在白名单中，但 -i 使其变为 Critical）
	if cmdName == "sed" && containsFlag(parts, "-i") {
		return &CommandInfo{
			RiskLevel:   RiskLevelCritical,
			Reason:      "sed -i 会直接修改文件内容",
			Explanation: "sed -i 命令会原地修改文件，可能导致文件损坏",
		}
	}

	// 6. 检查路径遍历（即使在安全命令中也要检查）
	for _, part := range parts[1:] {
		if strings.Contains(part, "..") {
			return &CommandInfo{
				RiskLevel:   RiskLevelHigh,
				Reason:      "命令包含路径遍历尝试",
				Explanation: "命令中包含路径遍历字符'..'，可能访问非预期的目录",
			}
		}
	}

	// 7. chmod / chown / mv 为 Medium
	if cmdName == "chmod" || cmdName == "chown" {
		return &CommandInfo{
			RiskLevel:   RiskLevelMedium,
			Reason:      cmdName + " 修改文件权限/所有者",
			Explanation: commandDescriptions[cmdName] + "，错误设置可能导致安全漏洞",
		}
	}
	if cmdName == "mv" {
		return &CommandInfo{
			RiskLevel:   RiskLevelMedium,
			Reason:      "mv 移动或重命名文件",
			Explanation: "mv命令用于移动或重命名文件，可能导致文件丢失或覆盖",
		}
	}

	// 7b. sysctl：修改内核运行时参数（带 = 或 -w）→ Medium；纯读取（如 sysctl vm.swappiness）→ Safe
	//     不写进 safeCommands 是因为 sysctl 写参数需要专门识别，不能整命令名一概放行。
	if cmdName == "sysctl" {
		return analyzeSysctl(parts)
	}

	// 7c. 多义工具子命令分级（docker / kubectl / systemctl 等）。
	// 这些工具不能整命令名放白名单 —— `docker rm` 是 High, `docker ps` 是 Safe。
	// 见 subCommandRisk 表注释。
	if table, ok := subCommandRisk[cmdName]; ok {
		return analyzeToolSubCommand(cmdName, parts, table)
	}

	// 8. 中等风险命令（curl/wget 等）
	if mediumRiskCommandsMap[cmdName] {
		return &CommandInfo{
			RiskLevel:   RiskLevelMedium,
			Reason:      cmdName + " 可能产生网络副作用或下载执行任意代码",
			Explanation: getDesc(cmdName) + "，需注意网络请求目标和下载内容安全性",
		}
	}

	// 9. 安全白名单命令
	if isSafeCommand(cmdName) {
		return &CommandInfo{
			RiskLevel:   RiskLevelSafe,
			Reason:      "命令在安全白名单中",
			Explanation: getDesc(cmdName),
		}
	}

	// 10. 未知命令
	// 改为 Medium 而非 Low：analyzer 不认识的命令应当至少要求用户确认一次。
	// Low 风险在 handler.go 中是直接执行不询问，对不认识命令过于乐观。
	// 安全优先：宁可多确认，不可漏确认。
	return &CommandInfo{
		RiskLevel:   RiskLevelMedium,
		Reason:      "命令不在白名单中，存在潜在风险",
		Explanation: getDesc(cmdName),
	}
}

// analyzeSysctl 分析 sysctl 命令。
//   - sysctl <name>           → Safe（读取参数）
//   - sysctl -n <name>        → Safe（只输出值）
//   - sysctl <name>=<value>   → Medium（修改内核运行时参数）
//   - sysctl -w <name>=<value> → Medium（显式写）
//   - sysctl --system         → Medium（从配置文件加载所有）
//   - sysctl -a / -A          → Safe（列出所有参数）
func analyzeSysctl(parts []string) *CommandInfo {
	// 任何带 -w / --write / --system 的都是写入
	for _, p := range parts[1:] {
		if p == "-w" || p == "--write" || p == "--system" || p == "--load" {
			return &CommandInfo{
				RiskLevel:   RiskLevelMedium,
				Reason:      "sysctl 修改内核运行时参数",
				Explanation: "sysctl 写入操作改变系统行为，可能影响性能/网络/内存等",
			}
		}
	}
	// 检查是否有 name=value 形式的赋值（无 -w 也可写：sysctl vm.swappiness=10 等价于 -w）
	for _, p := range parts[1:] {
		if strings.Contains(p, "=") && !strings.HasPrefix(p, "-") {
			return &CommandInfo{
				RiskLevel:   RiskLevelMedium,
				Reason:      "sysctl 修改内核运行时参数",
				Explanation: "sysctl " + p + " 修改内核参数，可能影响系统行为",
			}
		}
	}
	// 其余情况都是读
	return &CommandInfo{
		RiskLevel:   RiskLevelSafe,
		Reason:      "sysctl 只读查询",
		Explanation: "读取内核运行时参数，无副作用",
	}
}

// analyzeToolSubCommand 分析多义工具的子命令。
//   - 子命令在 readOnly 集合 → Safe
//   - 子命令在 mutating 集合 → mutatingSubCommandRiskLevel (High)
//   - 子命令未列出 → 走未知命令路径（Medium），避免新出现的高危子命令漏判
//
// 例：docker ps → Safe；docker inspect → Safe；docker rm → High；docker exec → High
//
//	jubectl get → Safe；kubectl delete → High
//	systemctl status → Safe；systemctl stop → High
func analyzeToolSubCommand(name string, parts []string, table struct {
	readOnly map[string]bool
	mutating map[string]bool
}) *CommandInfo {
	// 找第一个在分级表中出现的参数（无论它是 flag 还是子命令名）。
	// 设计考虑：
	//   - docker ps / kubectl get / systemctl status  → 子命令名形式
	//   - iptables -L / firewall-cmd --list-all / crontab -l  → flag 形式
	//   这两类都必须查到。docker --tls ps 这种全局 flag 不在表中，
	//   循环会继续往后找到真正的 ps。
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		// iptables/ip6tables 的 -t / --table 后面跟 table 名（filter/nat/mangle/raw/security），
		// 这个 table 名不是子命令，不能被当作未知子命令返回 Medium。
		// 跳过 -t / --table 及其值，让循环继续找真正的操作 flag（-A/-D/-L 等）。
		if p == "-t" || p == "--table" {
			i++ // 跳过 table 名
			continue
		}
		// -tfilter 这种连写形式，跳过即可（它本身就是 flag）
		// firewall-cmd --add-port=8080/tcp 这种 --flag=value 形式，
		// 剥离 =value 后再查表（只需匹配 --add-port 这个 flag 名）
		lookupKey := p
		if idx := strings.IndexByte(p, '='); idx > 0 {
			lookupKey = p[:idx]
		}
		// 单参数直接查
		if table.readOnly[lookupKey] {
			return &CommandInfo{
				RiskLevel:   RiskLevelSafe,
				Reason:      name + " " + lookupKey + " 是只读查询",
				Explanation: getDesc(name) + " " + lookupKey + "：只读操作",
			}
		}
		if table.mutating[lookupKey] {
			return &CommandInfo{
				RiskLevel:   mutatingSubCommandRiskLevel,
				Reason:      name + " " + lookupKey + " 变更运行中的资源",
				Explanation: name + " " + lookupKey + " 可能影响运行中的容器/服务/pod，需确认",
			}
		}
		// 复合子命令：尝试 parts[i]+parts[i+1] 组合
		// （kubectl cluster-info / docker network ls / ctr images ls）
		if i+1 < len(parts) {
			compound := p + " " + parts[i+1]
			if table.readOnly[compound] {
				return &CommandInfo{
					RiskLevel:   RiskLevelSafe,
					Reason:      name + " " + compound + " 是只读查询",
					Explanation: name + " " + compound + "：只读操作",
				}
			}
			if table.mutating[compound] {
				return &CommandInfo{
					RiskLevel:   mutatingSubCommandRiskLevel,
					Reason:      name + " " + compound + " 变更运行中的资源",
					Explanation: name + " " + compound + " 可能影响运行中的容器/服务/pod",
				}
			}
		}
		// 三词复合子命令：尝试 parts[i]+parts[i+1]+parts[i+2] 组合
		// （ip xfrm policy add / ip xfrm state show / etcdctl member remove）
		if i+2 < len(parts) {
			triple := p + " " + parts[i+1] + " " + parts[i+2]
			if table.readOnly[triple] {
				return &CommandInfo{
					RiskLevel:   RiskLevelSafe,
					Reason:      name + " " + triple + " 是只读查询",
					Explanation: name + " " + triple + "：只读操作",
				}
			}
			if table.mutating[triple] {
				return &CommandInfo{
					RiskLevel:   mutatingSubCommandRiskLevel,
					Reason:      name + " " + triple + " 变更运行中的资源",
					Explanation: name + " " + triple + " 可能影响运行中的容器/服务/pod",
				}
			}
		}
		// 非 flag 的位置参数若是未知子命令 → Medium
		// (flag 形式的全局选项如 docker --tls 不在表中，继续往后找)
		if !strings.HasPrefix(p, "-") {
			return &CommandInfo{
				RiskLevel:   RiskLevelMedium,
				Reason:      name + " 的子命令 " + p + " 不在已知分级表中",
				Explanation: name + " " + p + "：未识别子命令，需确认后再执行",
			}
		}
		// 是 flag 但不在表中 → 可能是全局选项（--tls / -H），继续往后找
	}
	// 只有主命令无子命令（如 `docker` 不带参数）。
	// 多义工具光打 help/version 可能没事，但 flag 没匹配到分级表时
	// 必须保守 Medium（不能是 Low —— Low 在 handler 中直接执行不询问）。
	return &CommandInfo{
		RiskLevel:   RiskLevelMedium,
		Reason:      name + " 无子命令",
		Explanation: "仅执行 " + name + " 默认行为或未识别 flag，需确认后再执行",
	}
}

// analyzeCommandExecutor 分析 sh -c / bash -c / xargs / sudo 等命令执行器
func (a *CommandAnalyzer) analyzeCommandExecutor(name string, parts []string, fullCmd string) *CommandInfo {
	// 找到 -c 后的参数
	if name == "sh" || name == "bash" || name == "zsh" || name == "dash" || name == "ash" {
		// 找到 -c 后的参数
		for i := 1; i < len(parts); i++ {
			if parts[i] == "-c" && i+1 < len(parts) {
				subInfo := a.AnalyzeCommand(parts[i+1])
				// 命令执行器本身提升一级风险（因为绕过了直接调用）
				risk := subInfo.RiskLevel
				if risk < RiskLevelMedium {
					risk = RiskLevelMedium
				}
				return &CommandInfo{
					RiskLevel:   risk,
					Reason:      fmt.Sprintf("%s -c 执行子命令: %s", name, subInfo.Reason),
					Explanation: subInfo.Explanation,
				}
			}
		}
		// 无 -c 的 shell 调用，High — 可从管道接收并执行任意命令
		return &CommandInfo{
			RiskLevel:   RiskLevelHigh,
			Reason:      name + " 启动子 shell，可执行任意命令",
			Explanation: "启动新的 shell 进程，可能从管道或输入执行任意命令",
		}
	}

	// Windows shell 启动器：powershell / pwsh / cmd
	// - powershell -Command <cmd> / -c <cmd>  → 递归分析子命令
	// - cmd /c <cmd>                          → 递归分析子命令
	if name == "powershell" || name == "pwsh" || name == "cmd" {
		// powershell/pwsh 支持 -Command/-c；cmd 支持 /c（大小写不敏感）
		for i := 1; i < len(parts); i++ {
			arg := parts[i]
			lowerArg := strings.ToLower(arg)
			if (arg == "-Command" || arg == "-c") && i+1 < len(parts) {
				return a.analyzeWindowsShellChild(name, parts[i+1:])
			}
			if lowerArg == "/c" && i+1 < len(parts) {
				return a.analyzeWindowsShellChild(name, parts[i+1:])
			}
		}
		// 无 -Command / /c 的 shell 调用，High — 可从管道接收并执行任意命令
		return &CommandInfo{
			RiskLevel:   RiskLevelHigh,
			Reason:      name + " 启动子 shell，可执行任意命令",
			Explanation: "启动新的 shell 进程，可能从管道或输入执行任意命令",
		}
	}

	if name == "sudo" {
		// sudo -i/-s/--login/--shell 启动交互式 shell → High
		for _, p := range parts[1:] {
			if p == "-i" || p == "-s" || p == "--login" || p == "--shell" {
				return &CommandInfo{
					RiskLevel:   RiskLevelHigh,
					Reason:      "sudo 启动交互式 shell，可执行任意命令",
					Explanation: "sudo " + p + " 会以 root 身份打开 shell，风险极高",
				}
			}
		}
		// 找到第一个非 flag 参数作为子命令
		for i := 1; i < len(parts); i++ {
			if !strings.HasPrefix(parts[i], "-") {
				subCmd := strings.Join(parts[i:], " ")
				subInfo := a.analyzeSingleCommand(subCmd)
				risk := subInfo.RiskLevel
				if risk < RiskLevelMedium {
					risk = RiskLevelMedium
				}
				return &CommandInfo{
					RiskLevel:   risk,
					Reason:      fmt.Sprintf("sudo 配合: %s", subInfo.Reason),
					Explanation: subInfo.Explanation,
				}
			}
		}
		return &CommandInfo{
			RiskLevel:   RiskLevelMedium,
			Reason:      "sudo 执行",
			Explanation: "sudo 可能以 root 身份执行任意子命令",
		}
	}

	if name == "xargs" {
		// xargs -I/-L/-n/-P 等带值 flag 需跳过其值参数
		for i := 1; i < len(parts); i++ {
			p := parts[i]
			if strings.HasPrefix(p, "-") {
				// 带值 flag：跳过下一个参数
				if p == "-I" || p == "-L" || p == "-n" || p == "-P" ||
					p == "--replace" || p == "--max-lines" || p == "--max-args" || p == "--max-procs" {
					i++ // skip value
				}
				continue
			}
			// 第一个非 flag 参数是子命令
			subCmd := strings.Join(parts[i:], " ")
			subInfo := a.analyzeSingleCommand(subCmd)
			return &CommandInfo{
				RiskLevel:   subInfo.RiskLevel,
				Reason:      fmt.Sprintf("xargs 配合: %s", subInfo.Reason),
				Explanation: subInfo.Explanation,
			}
		}
		return &CommandInfo{
			RiskLevel:   RiskLevelMedium,
			Reason:      "xargs 执行",
			Explanation: "xargs 可能执行任意子命令",
		}
	}

	// eval / source / exec
	return &CommandInfo{
		RiskLevel:   RiskLevelHigh,
		Reason:      name + " 可执行任意命令",
		Explanation: name + " 可以执行或加载任意命令/脚本，需仔细确认内容",
	}
}

// analyzeWindowsShellChild 分析 PowerShell -Command / cmd /c 后的子命令字符串。
// parts 是 -Command / /c 之后的全部参数，可能为多段（被 shlex 拆分的引号内容）。
func (a *CommandAnalyzer) analyzeWindowsShellChild(name string, parts []string) *CommandInfo {
	if len(parts) == 0 {
		return &CommandInfo{
			RiskLevel:   RiskLevelMedium,
			Reason:      name + " -Command 无内容",
			Explanation: "启动 shell 但未指定要执行的命令",
		}
	}
	// 子命令可能被 shlex 拆成多段（原本是引号包裹的单字符串），重新拼接后递归分析
	subCmd := strings.Join(parts, " ")
	subInfo := a.AnalyzeCommand(subCmd)
	// 命令执行器本身提升一级风险（绕过了直接调用）
	risk := subInfo.RiskLevel
	if risk < RiskLevelMedium {
		risk = RiskLevelMedium
	}
	return &CommandInfo{
		RiskLevel:   risk,
		Reason:      fmt.Sprintf("%s -Command 执行子命令: %s", name, subInfo.Reason),
		Explanation: subInfo.Explanation,
	}
}

// analyzeFindExec 分析 find -exec 中的操作
func (a *CommandAnalyzer) analyzeFindExec(cmd string) *CommandInfo {
	dangerousInExec := []string{"rm", "mv", "chmod", "chown", "dd", "sh", "bash"}
	lowerCmd := strings.ToLower(cmd)
	for _, d := range dangerousInExec {
		if strings.Contains(lowerCmd, "-exec "+d) || strings.Contains(lowerCmd, "-execdir "+d) {
			return &CommandInfo{
				RiskLevel:   RiskLevelCritical,
				Reason:      "find 命令包含危险的 " + d + " 操作",
				Explanation: "该命令使用find配合" + d + "，可能会误操作大量文件",
			}
		}
	}
	return nil
}

// hasWriteRedirect 检查是否包含真正的文件写入重定向。
// 排除以下安全场景：
//   - 2>/dev/null / 2>&1（stderr 丢弃或合并，不写文件）
//   - > /dev/null / >> /dev/null（输出丢弃，不写文件）
//   - < file（输入重定向，不写文件）
//   - Windows: 2>NUL / >NUL / $null 同理丢弃，不写文件
func hasWriteRedirect(cmd string) bool {
	// 移除所有 null 设备重定向（Linux /dev/null 和 Windows NUL，均为丢弃输出不写文件）
	cleaned := strings.ReplaceAll(cmd, "/dev/null", "")
	// Windows NUL 设备（大小写不敏感：NUL / nul）
	cleaned = regexp.MustCompile(`(?i)\bNUL\b`).ReplaceAllString(cleaned, "")
	// PowerShell $null（输出丢弃）
	cleaned = strings.ReplaceAll(cleaned, "$null", "")
	// 移除输入重定向 < file
	reInput := regexp.MustCompile(`<\s*\S+`)
	cleaned = reInput.ReplaceAllString(cleaned, "")
	// 移除 2>&1 合并重定向
	cleaned = strings.ReplaceAll(cleaned, "2>&1", "")
	// 移除 2> stderr 重定向（2> 后面如果已被清理说明是丢弃）
	reStderr := regexp.MustCompile(`2>\s*`)
	cleaned = reStderr.ReplaceAllString(cleaned, "")

	// 现在检查剩余是否有 > 或 >>（真正的文件写入）
	// 用正则匹配 > 或 >> 后面跟非空内容（文件名）
	reWrite := regexp.MustCompile(`>>?\s*\S+`)
	return reWrite.MatchString(cleaned)
}

// containsFlag 检查参数中是否包含指定 flag
func containsFlag(parts []string, flag string) bool {
	for _, p := range parts {
		if p == flag {
			return true
		}
	}
	return false
}

// getDesc 获取命令描述
func getDesc(name string) string {
	if desc, ok := commandDescriptions[name]; ok {
		return desc
	}
	return "执行系统命令"
}
