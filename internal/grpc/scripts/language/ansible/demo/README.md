# Ansible Roles Demo

这是一个可直接导入 Codebook 项目的 Ansible Roles 示例，不依赖远程主机或 SSH 凭据。

## ETask 测试参数

- 程序入口：`site.yml`（如果导入后保留了 `demo` 顶层目录，则使用 `demo/site.yml`）
- YAML Inventory：`inventory/hosts.yml`
- INI Inventory：`inventory/hosts.ini`
- 可选变量：`demo_environment`、`demo_message`
- 可选 Tags：`validate`、`render`

Inventory 使用 `localhost` 和 `ansible_connection: local`，可以安全验证 Playbook、Role、
defaults、group_vars、files、templates、handlers、tags 和 Extra Vars。

本地验证命令：

```bash
ansible-playbook --inventory inventory/hosts.yml site.yml
ansible-playbook --inventory inventory/hosts.ini site.yml
ansible-playbook --inventory inventory/hosts.yml --tags validate site.yml
```
