package multiverse

import (
	"strings"

	"github.com/iotaledger/hive.go/datastructure/randommap"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/multivers-simulation/config"
)

var (
	OptimalStrongTipsCount = int(float64(config.TipsCount) * (1 - config.WeakTipsRatio))
	OptimalWeakTipsCount   = int(float64(config.TipsCount) * config.WeakTipsRatio)
)

// region TipManager ///////////////////////////////////////////////////////////////////////////////////////////////////

type TipManager struct {
	Events *TipManagerEvents

	tangle              *Tangle
	tsa                 TipSelector
	tipSets             map[Color]*TipSet
	msgProcessedCounter map[Color]uint64
}

func NewTipManager(tangle *Tangle, tsaString string) (tipManager *TipManager) {
	tsaString = strings.ToUpper(tsaString) // make sure string is upper case
	var tsa TipSelector
	switch tsaString {
	case "URTS":
		tsa = URTS{}
	case "RURTS":
		tsa = RURTS{}
	default:
		tsa = URTS{}
	}

	// Initialize the counters
	msgProcessedCounter := make(map[Color]uint64)
	msgProcessedCounter[UndefinedColor] = 0
	msgProcessedCounter[Red] = 0
	msgProcessedCounter[Green] = 0

	return &TipManager{
		Events: &TipManagerEvents{
			MessageProcessed: events.NewEvent(messageProcessedHandler),
		},

		tangle:              tangle,
		tsa:                 tsa,
		tipSets:             make(map[Color]*TipSet),
		msgProcessedCounter: msgProcessedCounter,
	}
}

func (t *TipManager) Setup() {
	t.tangle.OpinionManager.Events.OpinionFormed.Attach(events.NewClosure(t.AnalyzeMessage))
}

func (t *TipManager) AnalyzeMessage(messageID MessageID) {
	message := t.tangle.Storage.Message(messageID)
	messageMetadata := t.tangle.Storage.MessageMetadata(messageID)
	inheritedColor := messageMetadata.InheritedColor()
	tipSet := t.TipSet(inheritedColor)
	// Calculate the current tip pool size before calling AddStrongTip
	current_tip_pool_size := tipSet.strongTips.Size()

	addedAsStrongTip := make(map[Color]bool)
	for color, tipSet := range t.TipSets(inheritedColor) {
		addedAsStrongTip[color] = true
		tipSet.AddStrongTip(message)
		t.msgProcessedCounter[color] += 1
	}

	// Color, tips pool count, processed messages issued messages
	t.Events.MessageProcessed.Trigger(inheritedColor, current_tip_pool_size,
		t.msgProcessedCounter[inheritedColor], messageIDCounter)

	// Remove the weak tip codes
	// for color, tipSet := range t.TipSets(inheritedColor) {
	// 	if !addedAsStrongTip[color] {
	// 		tipSet.AddWeakTip(message)
	// 	}
	// }
}

func (t *TipManager) TipSets(color Color) map[Color]*TipSet {
	if _, exists := t.tipSets[color]; !exists {
		t.tipSets[color] = NewTipSet(t.tipSets[UndefinedColor])
	}

	if color == UndefinedColor {
		return t.tipSets
	}

	return map[Color]*TipSet{
		color: t.tipSets[color],
	}
}

func (t *TipManager) TipSet(color Color) (tipSet *TipSet) {
	tipSet, exists := t.tipSets[color]
	if !exists {
		tipSet = NewTipSet(t.tipSets[UndefinedColor])
		t.tipSets[color] = tipSet
	}

	return
}

func (t *TipManager) Tips() (strongTips MessageIDs, weakTips MessageIDs) {
	tipSet := t.TipSet(t.tangle.OpinionManager.Opinion())

	strongTips = tipSet.StrongTips(config.TipsCount, t.tsa)
	// In the paper we consider all strong tips
	// weakTips = tipSet.WeakTips(config.TipsCount-1, t.tsa)

	// if len(weakTips) == 0 {
	// 	return
	// }

	// if strongTipsCount := len(strongTips); strongTipsCount < OptimalStrongTipsCount {
	// 	fillUpCount := config.TipsCount - strongTipsCount

	// 	if fillUpCount >= len(weakTips) {
	// 		return
	// 	}

	// 	weakTips.Trim(fillUpCount)
	// 	return
	// }

	// if weakTipsCount := len(weakTips); weakTipsCount < OptimalWeakTipsCount {
	// 	fillUpCount := config.TipsCount - weakTipsCount

	// 	if fillUpCount >= len(strongTips) {
	// 		return
	// 	}

	// 	strongTips.Trim(fillUpCount)
	// 	return
	// }

	strongTips.Trim(OptimalStrongTipsCount)
	// weakTips.Trim(OptimalWeakTipsCount)

	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TipSet ///////////////////////////////////////////////////////////////////////////////////////////////////////

type TipSet struct {
	strongTips *randommap.RandomMap
	weakTips   *randommap.RandomMap
}

func NewTipSet(tipsToInherit *TipSet) (tipSet *TipSet) {
	tipSet = &TipSet{
		strongTips: randommap.New(),
		weakTips:   randommap.New(),
	}

	if tipsToInherit != nil {
		tipsToInherit.strongTips.ForEach(func(key interface{}, value interface{}) {
			tipSet.strongTips.Set(key, value)
		})
		tipsToInherit.weakTips.ForEach(func(key interface{}, value interface{}) {
			tipSet.weakTips.Set(key, value)
		})
	}

	return
}

func (t *TipSet) AddStrongTip(message *Message) {
	t.strongTips.Set(message.ID, message)

	for strongParent := range message.StrongParents {
		t.strongTips.Delete(strongParent)
	}

	for weakParent := range message.WeakParents {
		t.weakTips.Delete(weakParent)
	}
}

func (t *TipSet) AddWeakTip(message *Message) {
	t.weakTips.Set(message.ID, message)
}

func (t *TipSet) StrongTips(maxAmount int, tsa TipSelector) (strongTips MessageIDs) {
	if t.strongTips.Size() == 0 {
		strongTips = NewMessageIDs(Genesis)
		return
	}

	strongTips = make(MessageIDs)
	for _, strongTip := range tsa.TipSelect(t.strongTips, maxAmount) {
		strongTips.Add(strongTip.(*Message).ID)
	}

	return
}

func (t *TipSet) WeakTips(maxAmount int, tsa TipSelector) (weakTips MessageIDs) {
	if t.weakTips.Size() == 0 {
		return
	}

	weakTips = make(MessageIDs)
	for _, weakTip := range tsa.TipSelect(t.weakTips, maxAmount) {
		weakTips.Add(weakTip.(*Message).ID)
	}

	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TipSelector //////////////////////////////////////////////////////////////////////////////////////////////////

// TipSelector defines the interface for a TSA
type TipSelector interface {
	TipSelect(tips *randommap.RandomMap, maxAmount int) []interface{}
}

// URTS implements the uniform random tip selection algorithm
type URTS struct {
	TipSelector
}

// RURTS implements the restricted uniform random tip selection algorithm, where txs are only valid tips up to some age D
type RURTS struct {
	TipSelector
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// TipSelect selects maxAmount tips
func (URTS) TipSelect(tips *randommap.RandomMap, maxAmount int) []interface{} {
	return tips.RandomUniqueEntries(maxAmount)

}

// region TipSelect Algorithm /////////////////////////////////////////////////////////////////////////////////////////
// TipSelect selects maxAmount tips
// TODO: Modify this tip selection algorithm
// RURTS: URTS with max parent age restriction
func (RURTS) TipSelect(tips *randommap.RandomMap, maxAmount int) []interface{} {
	return tips.RandomUniqueEntries(maxAmount)

}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region TipManagerEvents /////////////////////////////////////////////////////////////////////////////////////////

type TipManagerEvents struct {
	MessageProcessed *events.Event
}

func messageProcessedHandler(handler interface{}, params ...interface{}) {
	handler.(func(Color, int, uint64, int64))(params[0].(Color), params[1].(int), params[2].(uint64), params[3].(int64))
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
