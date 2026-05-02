package main

import (
	"os"

	hitkeepcmd "hitkeep/cmd"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "recover" {
		hitkeepcmd.Recover()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "update-spam-lists" {
		hitkeepcmd.UpdateSpamLists(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "import" {
		hitkeepcmd.Import(os.Args[2:])
		return
	}
	hitkeepcmd.Run()
}
