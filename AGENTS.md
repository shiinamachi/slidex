# Communication

- Always ask questions and respond to the user in Korean.
- Use English for internal reasoning and intermediate reasoning.

# Git Workflow

- Agents must create a Git commit directly after each small, coherent change.
- Keep commits narrowly scoped to the change just completed.
- Do not include unrelated user changes or generated artifacts unless they are required for the task.
- Write commit messages using Conventional Commits.
- Prefer concise commit messages, for example `docs: update agent instructions` or `fix: handle missing broker config`.

# Model Selection

- Use GPT-5.5 Xhigh for analysis, investigation, architecture, and design work.
- Use GPT-5.5 High for development, implementation, refactoring, and test fixes.
- When spawning sub-agents, choose GPT-5.5 Xhigh or GPT-5.5 High according to the task difficulty and type.

# Sub-Agent Usage

- Use sub-agents for work that can be performed in parallel.
- Prefer assigning sub-agents concrete, independent tasks with clear ownership.
- For code changes, define the files or modules each sub-agent owns.
- Sub-agents must not revert or overwrite changes made by other agents or the user.
- Integrate sub-agent results carefully before committing.
