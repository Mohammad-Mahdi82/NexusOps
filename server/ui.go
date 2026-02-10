package main

import (
	"fmt"
	"sort"

	"github.com/Mohammad-Mahdi82/NexusOps/server/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/shopspring/decimal"
)

func (s *server) refreshUI() {
	s.app.QueueUpdateDraw(func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.mainFlex.Clear()
		s.pcTables = nil

		var connectedPCs []string
		for id := range s.pcStates {
			connectedPCs = append(connectedPCs, id)
		}
		sort.Strings(connectedPCs)

		if len(connectedPCs) == 0 {
			emptyMsg := tview.NewTextView().SetText("\n\nNo PCs Connected.").SetTextAlign(tview.AlignCenter)
			s.mainFlex.AddItem(emptyMsg, 0, 1, false)
			return
		}

		for _, pcID := range connectedPCs {
			pcCol := tview.NewFlex().SetDirection(tview.FlexRow)
			pcCol.SetBorder(true).SetTitle(fmt.Sprintf(" %s ", pcID)).SetBorderAttributes(tcell.AttrBold).SetBorderPadding(0, 0, 1, 1)

			table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
			table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorNone).Foreground(tcell.ColorGreen))
			table.SetTitle(pcID)

			table.SetCell(0, 0, tview.NewTableCell("GAME").SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
			table.SetCell(0, 1, tview.NewTableCell("MIN").SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
			table.SetCell(0, 2, tview.NewTableCell("FEE").SetTextColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))

			var sessions []models.Session
			s.db.Where("pc_id = ? AND paid = ?", pcID, false).Order("start_time asc").Find(&sessions)

			subTotal := decimal.Zero
			row := 1
			for _, sess := range sessions {
				color := tcell.ColorGreen
				if !sess.IsActive {
					color = tcell.ColorGray
				}
				table.SetCell(row, 0, tview.NewTableCell(sess.GameName).SetTextColor(color))
				table.SetCell(row, 1, tview.NewTableCell(fmt.Sprintf("%d", sess.DurationMinutes)).SetTextColor(color))
				table.SetCell(row, 2, tview.NewTableCell(sess.Fee.StringFixed(0)).SetTextColor(color))
				subTotal = subTotal.Add(sess.Fee)
				row++
			}

			footerTable := tview.NewTable().SetBorders(false)
			footerTable.SetCell(0, 0, tview.NewTableCell(" TOTAL").SetTextColor(tcell.ColorBlack).SetBackgroundColor(tcell.ColorYellow).SetAttributes(tcell.AttrBold))
			footerTable.SetCell(0, 1, tview.NewTableCell(subTotal.StringFixed(0)+" ").SetTextColor(tcell.ColorBlack).SetBackgroundColor(tcell.ColorYellow).SetAlign(tview.AlignRight).SetExpansion(1))

			pcCol.AddItem(table, 0, 1, true)
			pcCol.AddItem(footerTable, 1, 0, false)

			pcCol.SetFocusFunc(func() { pcCol.SetBorderColor(tcell.ColorYellow) })
			pcCol.SetBlurFunc(func() { pcCol.SetBorderColor(tcell.ColorWhite) })

			s.pcTables = append(s.pcTables, table)
			s.mainFlex.AddItem(pcCol, 32, 0, true)
		}
		s.mainFlex.AddItem(nil, 0, 1, false)
		if len(s.pcTables) > 0 {
			s.app.SetFocus(s.pcTables[0])
		}
	})
}
