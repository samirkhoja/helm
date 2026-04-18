package main

import (
	"runtime"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
)

func sessionAlternateAccelerators() (
	previousLabel string,
	previousShortcut *keys.Accelerator,
	nextLabel string,
	nextShortcut *keys.Accelerator,
) {
	if runtime.GOOS == "darwin" {
		return "Previous Session (Cmd+Option+Left)",
			keys.Combo("left", keys.CmdOrCtrlKey, keys.OptionOrAltKey),
			"Next Session (Cmd+Option+Right)",
			keys.Combo("right", keys.CmdOrCtrlKey, keys.OptionOrAltKey)
	}

	return "Previous Session (Ctrl+Shift+Tab)",
		keys.Combo("tab", keys.ControlKey, keys.ShiftKey),
		"Next Session (Ctrl+Tab)",
		keys.Control("tab")
}

func buildMenu(app *App) *menu.Menu {
	root := menu.NewMenu()
	root.Append(menu.AppMenu())

	shellMenu := root.AddSubmenu("Shell")
	shellMenu.AddText("New Workspace", keys.CmdOrCtrl("o"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionNewWorkspace)
	})
	shellMenu.AddText("New Session", keys.CmdOrCtrl("t"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionNewSession)
	})
	shellMenu.AddText("Close Session", keys.CmdOrCtrl("w"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionCloseSession)
	})
	shellMenu.AddText("Command Palette", keys.CmdOrCtrl("p"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionCommandPalette)
	})
	shellMenu.AddSeparator()
	shellMenu.AddText("Save File", keys.CmdOrCtrl("s"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionSaveFileEditor)
	})
	shellMenu.AddSeparator()
	shellMenu.AddText("Toggle Left Rail", keys.CmdOrCtrl("b"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionToggleSidebar)
	})
	shellMenu.AddText("Toggle Diff Panel", keys.Combo("d", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionToggleDiff)
	})
	shellMenu.AddText("Toggle Files Panel", keys.Combo("e", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionToggleFiles)
	})
	shellMenu.AddText("Toggle Shell Panel", nil, func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionToggleShell)
	})
	shellMenu.AddText("Toggle Peers Panel", keys.Combo("p", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionTogglePeers)
	})
	shellMenu.AddText("Toggle Diff Fullscreen", keys.Combo("f", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionToggleDiffFullscreen)
	})
	shellMenu.AddText("Focus Terminal", keys.CmdOrCtrl("1"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionFocusTerminal)
	})
	shellMenu.AddText("Focus Files Panel", keys.CmdOrCtrl("2"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionFocusFilesPanel)
	})
	shellMenu.AddSeparator()
	shellMenu.AddText("Zoom Out Terminal Text", keys.CmdOrCtrl("-"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionZoomOutTerminal)
	})
	shellMenu.AddText("Reset Terminal Text Zoom", keys.CmdOrCtrl("0"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionResetTerminalZoom)
	})
	shellMenu.AddText("Zoom In Terminal Text", keys.CmdOrCtrl("="), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionZoomInTerminal)
	})
	shellMenu.AddSeparator()
	shellMenu.AddText("Refresh Diff", keys.Combo("r", keys.CmdOrCtrlKey, keys.OptionOrAltKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionRefreshDiff)
	})
	shellMenu.AddText("Zoom Out Diff Text", keys.Combo("-", keys.CmdOrCtrlKey, keys.OptionOrAltKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionZoomOutDiff)
	})
	shellMenu.AddText("Reset Diff Text Zoom", keys.Combo("0", keys.CmdOrCtrlKey, keys.OptionOrAltKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionResetDiffZoom)
	})
	shellMenu.AddText("Zoom In Diff Text", keys.Combo("=", keys.CmdOrCtrlKey, keys.OptionOrAltKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionZoomInDiff)
	})
	shellMenu.AddSeparator()
	shellMenu.AddText("Previous Session", keys.Combo("[", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionPreviousSession)
	})
	shellMenu.AddText("Next Session", keys.Combo("]", keys.CmdOrCtrlKey, keys.ShiftKey), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionNextSession)
	})

	previousSessionLabel, previousSessionShortcut, nextSessionLabel, nextSessionShortcut :=
		sessionAlternateAccelerators()
	shellMenu.AddText(previousSessionLabel, previousSessionShortcut, func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionPreviousSessionAlt)
	})
	shellMenu.AddText(nextSessionLabel, nextSessionShortcut, func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionNextSessionAlt)
	})
	shellMenu.AddSeparator()
	shellMenu.AddText("Dismiss Overlay or Panel", keys.Key("escape"), func(_ *menu.CallbackData) {
		app.emitMenuAction(menuActionDismissOverlay)
	})

	root.Append(menu.EditMenu())
	root.Append(menu.WindowMenu())

	return root
}
