# Halaqaty diagrams

Open the HTML files in a browser. JSON files are editable Archify sources.

- [Core learning journey](core-learning-journey.html)
- [System map](system-map.html)
- [Complete workflow: both interactive views on one page](feature-workflow.html)
- [Workflow view 1: scope and planning](feature-delivery.html)
- [Workflow view 2: implementation and review](implementation-review.html)

The two workflow views form one guide: view 1 ends at the approved-task handoff;
view 2 begins there and covers implementation, checks, review and merge.
Short box labels are explained in the cards below each diagram.
The combined page embeds both views for portability and retains their independent
interactive controls. Rebuild it after rendering changes with
`node docs/engineering/architecture/diagrams/combine-workflow.cjs` from the repo root.

Rules remain in the [workflow harness](../../collaboration/AGENT_WORKFLOW_HARNESS.md)
and [constitution](../../../../.specify/memory/constitution.md).
The small-fix routing clarification, PR evidence summary and required CI check
suggestions are proposals for those governing documents. These diagrams do not
change policy or configure CI, branch protection, or F-008 tasks.

These are explanatory process views, not evidence that a feature or gate passed.
Update the relevant source JSON when the process changes, then render with the
installed Archify skill. Generation receipts are local tooling output.
