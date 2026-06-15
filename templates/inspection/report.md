# Linux 服务器日常巡检报告

## 一、 报告概要

### 项目信息

| 项目 | 内容 |
| --- | --- |
| 报告名称 | Linux 服务器日常巡检报告 |
| 主机名 | {{hostname}} |
| IP 地址 | {{ip_address}} |
| 巡检时间 | {{inspection_time}} |
| 操作系统 | {{os_info}} |
| 内核版本 | {{kernel_version}} |
| 运行时间 | {{uptime_info}} |
| 巡检人员 | 运维人员 |
| 总体状态 | ☐ 健康 / ☐ 亚健康 / ☐ 故障 (高风险) |

## 二、 巡检详情

### 1. 硬件与系统健康

| 检查类别 | 检查项 | 命令/参考 | 当前值 | 状态 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 硬件 | CPU 型号/核心数 | cat /proc/cpuinfo | {{cpu_info}} | ☐ 正常 | |
| 硬件 | 内存总量 | free -h | {{mem_info}} | ☐ 正常 | |
| 硬件 | 硬盘健康 | smartctl -a /dev/sda 或 查看 dmesg | 无坏道/无报错 | ☐ 正常 | |
| 硬件 | RAID 卡状态 | hpssacli / MegaCli | 正常 | ☐ 正常 | 若为物理机 |
| 资源 | 磁盘使用率 | df -h | {{disk_usage}} | ☐ 正常 | |
| 资源 | 磁盘 Inode | df -i | /data (5%) | ☐ 正常 | |
| 资源 | 内存使用率 | free -m | {{mem_usage}} | ☐ 正常 | |
| 资源 | CPU 负载 | uptime | {{cpu_load}} | ☐ 正常 | |
| 资源 | SWAP 使用率 | free -m | {{swap_usage}} | ☐ 正常 | |
| 性能 | 磁盘 I/O 等待 | iostat -x 1 5 | %util < 10% | ☐ 正常 | |

### 2. 安全审计与日志分析

| 检查类别 | 检查项 | 命令/参考 | 当前值 | 状态 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 认证审计 | 登录失败统计 | lastb | wc -l 或 grep "Failed password" /var/log/secure | {{login_failures}} | ☐ 正常 | |
| 认证审计 | 近期登录记录 | last | 正常 | ☐ 正常 | |
| 认证审计 | 特权用户 | awk -F: '$3==0{print $1}' /etc/passwd | root | ☐ 正常 | 检查是否有非root的UID 0账户 |
| 认证审计 | 空密码账户 | awk -F: 'length($2)==0' /etc/shadow | 无 | ☐ 正常 | |
| 日志监控 | auditd 服务 | systemctl status auditd | active (running) | ☐ 正常 | 审计服务运行中 |
| 日志监控 | 日志完整性 | ls -lh /var/log/messages /var/log/secure | 正常轮转 | ☐ 正常 | |
| 日志监控 | 日志空间 | df -h /var/log | 已用 30% | ☐ 正常 | |
| 日志监控 | 系统错误 | grep -i error /var/log/messages | tail -20 | {{system_error}} | ☐ 正常 | |

### 3. 网络与服务

| 检查类别 | 检查项 | 命令/参考 | 当前值 | 状态 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 网络 | 防火墙规则 | iptables -L -n 或 firewall-cmd --list-all | 规则已加载 | ☐ 正常 | 确认规则是否符合预期 |
| 网络 | 网络连接数 | netstat -anp | grep ESTABLISHED | wc -l | 250 | ☐ 正常 | |
| 网络 | 网卡丢包 | ifconfig eth0 | grep dropped | 无丢包 | ☐ 正常 | |
| 服务 | 关键服务状态 | systemctl status sshd crond rsyslog | 均为 running | ☐ 正常 | |
| 服务 | 异常端口监听 | netstat -tulnp | 查看非预期端口 | 无 | ☐ 正常 | |
| 服务 | 计划任务 | crontab -l (以及 /etc/crontab) | 无异常任务 | ☐ 正常 | 检查是否有恶意定时任务 |

## 三、 发现的问题与风险清单

此部分直接列出本次巡检中发现的异常项，是报告的核心价值所在。

{{issues_list}}

## 四、 处理建议与优化方案

针对上述问题，给出具体的解决思路。

{{recommendations}}

## 五、 巡检结论

结论： 服务器当前处于 {{overall_status}} 状态。核心业务服务运行正常，{{conclusion_details}}。