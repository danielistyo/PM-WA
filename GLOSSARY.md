# PM-WA

A WhatsApp task-list bot: it manages task lists in group chats via text
commands, sends reminders, and exposes a private web dashboard per account.
All user-facing times are in GMT+7.

## Language

### People & identity

**Account**:
One linked WhatsApp bot identity with its own isolated data directory.
_Avoid_: bot instance, session

**Admin**:
The person who created a task list; the only one who can publish or stop it.
_Avoid_: owner

**Assignee**:
A person responsible for a task.
_Avoid_: user, member

### Lists & tasks

**Task List**:
A named set of tasks belonging to one group and one admin.
_Avoid_: list, checklist

**Task**:
A single item on a task list, identified by its 1-based position.
_Avoid_: item, todo

**Position**:
The 1-based number that identifies a task within its task list.
_Avoid_: index, number

**Overdue**:
A task whose deadline has passed and is not done.
_Avoid_: late

### List lifecycle

**Unpublished**:
A task list created but not yet visible to the group.
_Avoid_: draft

**Publish**:
Make a task list active and post its summary to the group.
_Avoid_: activate, start

**Active**:
A published task list that is live.
_Avoid_: open

**Stop**:
Permanently retire a task list; it can never be published again.
_Avoid_: close, archive

### Reminders

**Reminder**:
A scheduled message about a task, driven by one of four modes: none,
deadline, at, cron.
_Avoid_: alarm, notification, pulse

### Messages

**Command**:
A message sent to the bot privately that starts with a command word
(e.g. add-list, publish-list).
_Avoid_: request

**Reply**:
A group message answering a summary or reminder, like done: 2 or todo: 3.
_Avoid_: response

**Summary**:
The group message listing a task list's tasks with a progress bar.
_Avoid_: digest, report
