# mkx

Make target runner TUI with k9s-style navigation.

Discover projects in a workspace, browse their Make targets, and run them with full terminal handover.

## Features (MVP)

- Recursive Makefile discovery with configurable depth and exclusions
- Project list / target list navigation (j/k, Enter, Esc)
- Target descriptions from `## comment` convention
- Full terminal handover via `tea.ExecProcess` — interactive prompts work natively
- Status bar with exit code and duration

## Install

```bash
go install github.com/Gaetan-Jaminon/mkx@latest
```

Or build from source:

```bash
git clone https://github.com/Gaetan-Jaminon/mkx.git
cd mkx
go build -o mkx .
```

## Usage

```bash
cd ~/workspace
mkx
```

## Key Bindings

| Key | Project List | Target List |
|-----|-------------|-------------|
| j/k, arrows | Navigate | Navigate |
| Enter | Drill into project | Run target |
| r | - | Run target |
| / | Filter | Filter |
| ? | View README | View README |
| Esc | - | Back |
| q | Quit | Quit |

## Stack

- [Go](https://go.dev)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
