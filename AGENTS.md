## 提交粒度与时机
- 完成一个「可独立验证的改动单元」就提交，不要攒到任务结束。
- 一个单元 = 编译/语法通过 + 相关测试通过 + 单独存在不破坏仓库。
- 多步任务先列 TODO，每完成一项提交一次。
- 重构与行为变更必须分开提交。
- 禁止 `git add -A` / `git add .`，显式指定路径。
- 强制提交触发点：写完一个通过的测试、实现完一个函数、修完一个 bug、
  完成一次纯重构。
- 提交前自问：如果人类只想回退这次改动的一半，能做到吗？不能就拆。

## Agent skills

### Issue tracker

Issues 放在本仓库的 GitHub Issues，通过 `gh` CLI 读写。详见 `docs/agents/issue-tracker.md`。

### Triage labels

使用默认五类角色标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。详见 `docs/agents/triage-labels.md`。

### Domain docs

单上下文：根目录 `CONTEXT.md` + `docs/adr/`。详见 `docs/agents/domain.md`。
