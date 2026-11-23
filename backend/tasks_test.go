package backend

import (
	"fmt"
	"strings"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

func TestTaskSchedulerCOMDetails(t *testing.T) {
	if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
		ole.CoInitialize(0)
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		t.Fatalf("Failed to create Schedule.Service: %v", err)
	}
	defer unknown.Release()

	ts, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		t.Fatalf("QueryInterface failed: %v", err)
	}
	defer ts.Release()

	if _, err := oleutil.CallMethod(ts, "Connect"); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	rootFolderRes, err := oleutil.CallMethod(ts, "GetFolder", `\`)
	if err != nil {
		t.Fatalf("GetFolder failed: %v", err)
	}
	rootFolder := rootFolderRes.ToIDispatch()
	defer rootFolder.Release()

	var scanFolder func(folder *ole.IDispatch, path string)
	scanFolder = func(folder *ole.IDispatch, path string) {
		if strings.HasPrefix(strings.ToLower(path), `\microsoft\windows`) {
			return
		}

		// Tasks in this folder
		tasksRes, err := oleutil.CallMethod(folder, "GetTasks", 1) // 1 = TASK_ENUM_HIDDEN
		if err == nil {
			tasksCol := tasksRes.ToIDispatch()
			countVar, _ := oleutil.GetProperty(tasksCol, "Count")
			count := int(countVar.Val)
			for i := 1; i <= count; i++ {
				taskRes, err := oleutil.GetProperty(tasksCol, "Item", i)
				if err != nil {
					continue
				}
				task := taskRes.ToIDispatch()
				nameVar, _ := oleutil.GetProperty(task, "Name")
				name := nameVar.ToString()

				stateVar, _ := oleutil.GetProperty(task, "State")
				stateVal := int(stateVar.Val)
				state := "Ready"
				if stateVal == 1 {
					state = "Disabled"
				} else if stateVal == 4 {
					state = "Running"
				}

				lastRunStr := ""
				if lrVar, err := oleutil.GetProperty(task, "LastRunTime"); err == nil {
					if tVal, ok := lrVar.Value().(time.Time); ok && tVal.Year() > 2000 {
						lastRunStr = tVal.Format(time.RFC3339)
					}
				}

				// Actions & Triggers from Definition
				var execParts []string
				var triggerParts []string

				if defVar, err := oleutil.GetProperty(task, "Definition"); err == nil {
					def := defVar.ToIDispatch()

					// Actions
					if actColVar, err := oleutil.GetProperty(def, "Actions"); err == nil {
						actCol := actColVar.ToIDispatch()
						if aCountVar, err := oleutil.GetProperty(actCol, "Count"); err == nil {
							aCount := int(aCountVar.Val)
							for a := 1; a <= aCount; a++ {
								if aItemVar, err := oleutil.GetProperty(actCol, "Item", a); err == nil {
									aItem := aItemVar.ToIDispatch()
									pVar, _ := oleutil.GetProperty(aItem, "Path")
									argsVar, _ := oleutil.GetProperty(aItem, "Arguments")
									execParts = append(execParts, strings.TrimSpace(pVar.ToString()+" "+argsVar.ToString()))
									aItem.Release()
								}
							}
						}
						actCol.Release()
					}

					// Triggers
					if trigColVar, err := oleutil.GetProperty(def, "Triggers"); err == nil {
						trigCol := trigColVar.ToIDispatch()
						if tCountVar, err := oleutil.GetProperty(trigCol, "Count"); err == nil {
							tCount := int(tCountVar.Val)
							for tr := 1; tr <= tCount; tr++ {
								if tItemVar, err := oleutil.GetProperty(trigCol, "Item", tr); err == nil {
									tItem := tItemVar.ToIDispatch()
									typeVar, _ := oleutil.GetProperty(tItem, "Type")
									switch int(typeVar.Val) {
									case 1:
										triggerParts = append(triggerParts, "Event")
									case 2, 3, 4, 5, 6:
										triggerParts = append(triggerParts, "Time")
									case 7:
										triggerParts = append(triggerParts, "Idle")
									case 8:
										triggerParts = append(triggerParts, "Registration")
									case 9:
										triggerParts = append(triggerParts, "Boot")
									case 10:
										triggerParts = append(triggerParts, "Logon")
									case 11:
										triggerParts = append(triggerParts, "SessionStateChange")
									}
									tItem.Release()
								}
							}
						}
						trigCol.Release()
					}
					def.Release()
				}

				t.Logf("Task %s: State=%s, Exec=%q, Triggers=%v, LastRun=%s", name, state, strings.Join(execParts, " ; "), triggerParts, lastRunStr)
				task.Release()
			}
			tasksCol.Release()
		}

		// Subfolders
		subFoldersRes, err := oleutil.CallMethod(folder, "GetFolders", 0)
		if err == nil {
			subFoldersCol := subFoldersRes.ToIDispatch()
			countVar, _ := oleutil.GetProperty(subFoldersCol, "Count")
			count := int(countVar.Val)
			for i := 1; i <= count; i++ {
				subRes, err := oleutil.GetProperty(subFoldersCol, "Item", i)
				if err != nil {
					continue
				}
				sub := subRes.ToIDispatch()
				subPathVar, _ := oleutil.GetProperty(sub, "Path")
				subPath := subPathVar.ToString()
				scanFolder(sub, subPath)
				sub.Release()
			}
			subFoldersCol.Release()
		}
	}

	scanFolder(rootFolder, `\`)
	fmt.Println("COM Task details test passed!")
}
