package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jxherc/aivault/internal/wire"
)

var commands = []string{"init", "doctor", "sync", "import", "remove", "search", "add", "open", "watch", "autosync", "update", "completion", "mcp", "version", "help"}

func cmdCompletion(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: aivault completion bash|zsh|fish|powershell")
	}
	cmds, tools := strings.Join(commands, " "), strings.Join(wire.Names(), " ")
	switch args[0] {
	case "bash":
		fmt.Printf(`_aivault() {
  local cur=${COMP_WORDS[COMP_CWORD]} prev=${COMP_WORDS[1]}
  if [ "$COMP_CWORD" -eq 1 ]; then
    COMPREPLY=($(compgen -W "%s" -- "$cur")); return
  fi
  case "$prev" in
    remove) COMPREPLY=($(compgen -W "%s" -- "$cur")) ;;
    autosync) COMPREPLY=($(compgen -W "on off status" -- "$cur")) ;;
    completion) COMPREPLY=($(compgen -W "bash zsh fish powershell" -- "$cur")) ;;
    open) COMPREPLY=($(compgen -W "memory skills index library" -- "$cur")) ;;
  esac
}
complete -F _aivault aivault
`, cmds, tools)
	case "zsh":
		fmt.Printf(`#compdef aivault
_aivault() {
  if (( CURRENT == 2 )); then
    compadd -- %s; return
  fi
  case $words[2] in
    remove) compadd -- %s ;;
    autosync) compadd -- on off status ;;
    completion) compadd -- bash zsh fish powershell ;;
    open) compadd -- memory skills index library ;;
  esac
}
if (( $+functions[compdef] )); then compdef _aivault aivault; fi
`, cmds, tools)
	case "fish":
		fmt.Printf(`complete -c aivault -f
complete -c aivault -n __fish_use_subcommand -a "%s"
complete -c aivault -n "__fish_seen_subcommand_from remove" -a "%s"
complete -c aivault -n "__fish_seen_subcommand_from autosync" -a "on off status"
complete -c aivault -n "__fish_seen_subcommand_from completion" -a "bash zsh fish powershell"
complete -c aivault -n "__fish_seen_subcommand_from open" -a "memory skills index library"
`, cmds, tools)
	case "powershell":
		q := func(s string) string { return "'" + strings.ReplaceAll(s, " ", "','") + "'" }
		fmt.Printf(`Register-ArgumentCompleter -Native -CommandName aivault -ScriptBlock {
  param($word, $ast, $pos)
  $parts = $ast.CommandElements | ForEach-Object { $_.ToString() }
  $list = if ($parts.Count -le 2 -and $word) { @(%s) } elseif ($parts.Count -le 1) { @(%s) } else {
    switch ($parts[1]) {
      'remove' { @(%s) }
      'autosync' { @('on','off','status') }
      'completion' { @('bash','zsh','fish','powershell') }
      'open' { @('memory','skills','index','library') }
      default { @() }
    }
  }
  $list | Where-Object { $_ -like "$word*" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
}
`, q(cmds), q(cmds), q(tools))
	default:
		return fmt.Errorf("unknown shell %q. try bash, zsh, fish or powershell", args[0])
	}
	return nil
}
