package multiverse

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/datastructure/walker"

	"github.com/iotaledger/hive.go/datastructure/randommap"
	"github.com/iotaledger/hive.go/events"
	"github.com/iotaledger/multivers-simulation/config"
)

var (
	OptimalStrongParentsCount = int(float64(config.ParentsCount) * (1 - config.WeakTipsRatio))
	OptimalWeakParentsCount   = int(float64(config.ParentsCount) * config.WeakTipsRatio)
)

// region TipManager ///////////////////////////////////////////////////////////////////////////////////////////////////

type TipManager struct {
	Events *TipManagerEvents

	tangle *Tangle
	tsa    TipSelector

	tipSetsMutex sync.Mutex
	tipSets      map[Color]*TipSet

	msgProcessedMutex   sync.RWMutex
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
	//t.tangle.OpinionManager.Events().OpinionFormed.Attach(events.NewClosure(t.AnalyzeMessage))
	// Try "analysing" on scheduling instead of on opinion formation.
	t.tangle.Scheduler.Events().MessageScheduled.Attach(events.NewClosure(t.AnalyzeMessage))
}

func (t *TipManager) AnalyzeMessage(messageID MessageID) {
	message := t.tangle.Storage.Message(messageID)
	messageMetadata := t.tangle.Storage.MessageMetadata(messageID)
	inheritedColor := messageMetadata.InheritedColor()
	tipSet := t.TipSet(inheritedColor)
	// Calculate the current tip pool size before calling AddStrongTip
	currentTipPoolSize := tipSet.strongTips.Size()

	if time.Since(message.IssuanceTime).Seconds() < config.DeltaURTS || config.TSA != "RURTS" {
		addedAsStrongTip := make(map[Color]bool)
		for color, tipSet := range t.TipSets(inheritedColor) {
			addedAsStrongTip[color] = true
			tipSet.AddStrongTip(message)

			t.AddMsgProcessedCounter(color, 1)
		}
	}

	// Color, tips pool count, processed messages issued messages
	t.Events.MessageProcessed.Trigger(inheritedColor, currentTipPoolSize,
		t.GetMsgProcessedCounter(inheritedColor), messageIDCounter.Load())

	// Remove the weak tip codes
	// for color, tipSet := range t.TipSets(inheritedColor) {
	// 	if !addedAsStrongTip[color] {
	// 		tipSet.AddWeakTip(message)
	// 	}
	// }
}

func (t *TipManager) TipSets(color Color) map[Color]*TipSet {
	if color == UndefinedColor {
		return t.tipSets
	}

	t.tipSetsMutex.Lock()
	defer t.tipSetsMutex.Unlock()

	if _, exists := t.tipSets[color]; !exists {
		t.tipSets[color] = NewTipSet(t.tipSets[UndefinedColor])
	}

	return map[Color]*TipSet{
		color: t.tipSets[color],
	}
}

func (t *TipManager) TipSet(color Color) (tipSet *TipSet) {
	t.tipSetsMutex.Lock()
	defer t.tipSetsMutex.Unlock()

	tipSet, exists := t.tipSets[color]
	if !exists {
		tipSet = NewTipSet(t.tipSets[UndefinedColor])
		t.tipSets[color] = tipSet
	}

	return
}

func (t *TipManager) WalkForOldestUnconfirmed(tipSet *TipSet) (oldestMessage MessageID) {
	for _, tip := range tipSet.strongTips.Keys() {
		messageID := tip.(MessageID)
		//currentTangleTime := time.Now()
		//tipTangleTime := t.tangle.Storage.Message(messageID).IssuanceTime
		for latestAcceptedBlocks := range t.tangle.Storage.Message(messageID).StrongParents {
			if latestAcceptedBlocks == Genesis {
				continue
			}

			oldestUnconfirmedTime := time.Now()
			oldestConfirmationTime := time.Now()

			// Walk through the past cone to find the oldest unconfirmed blocks
			t.tangle.Utils.WalkMessagesAndMetadata(func(message *Message, messageMetadata *MessageMetadata, walker *walker.Walker) {
				confirmedTimestamp := messageMetadata.ConfirmationTime()
				// Reaches the confirmed blocks, stop traversing
				if !confirmedTimestamp.IsZero() {
					// Use the issuance time of the youngest confirmed block
					issuanceTime := message.IssuanceTime
					if issuanceTime.Before(oldestConfirmationTime) {
						oldestConfirmationTime = issuanceTime
					}
				} else {
					if message.IssuanceTime.Before(oldestUnconfirmedTime) {
						oldestUnconfirmedTime = message.IssuanceTime
						oldestMessage = message.ID
					}
					// Only continue the BFS when the current block is unconfirmed
					for strongChildID := range message.StrongParents {
						walker.Push(strongChildID)
					}
				}

			}, NewMessageIDs(messageID), false)

			//printAges(currentTangleTime, oldestUnconfirmedTime, oldestConfirmationTime, tipTangleTime)
			// if timeSinceConfirmation > tsc_condition {
			// 	oldTips[tip.(*Message).ID] = void{}
			// 	fmt.Printf("Prune %d\n", tip.(*Message).ID)
			// }
		}
	}
	return 0
}

func printAges(currentTangleTime time.Time, oldestUnconfirmedTime time.Time, oldestConfirmationTime time.Time, tipTangleTime time.Time) {
	// Distance between (Now, Issuance Time of the oldest UNCONFIRMED block that has confirmed parents)
	fmt.Printf("UnconfirmationAge %f\n", currentTangleTime.Sub(oldestUnconfirmedTime).Seconds())

	// Distance between (Now, Issuance Time of the oldest CONFIRMED block that has no confirmed children)
	fmt.Printf("ConfirmationAge %f\n", currentTangleTime.Sub(oldestConfirmationTime).Seconds())

	// Distance between (Issuance Time of the tip, Issuance Time of the oldest UNCONFIRMED block that has confirmed parents)
	fmt.Printf("UnconfirmationAgeSinceTip %f\n", tipTangleTime.Sub(oldestUnconfirmedTime).Seconds())

	// Distance between (Issuance Time of the tip, Issuance Time of the oldest CONFIRMED block that has no confirmed children)
	fmt.Printf("ConfirmationAgeSinceTip %f\n", tipTangleTime.Sub(oldestConfirmationTime).Seconds())

}

func (t *TipManager) Tips() (strongTips MessageIDs, weakTips MessageIDs) {
	// The tips is selected form the tipSet of the current ownOpinion
	tipSet := t.TipSet(t.tangle.OpinionManager.Opinion())

	// monitored peerID for TSC and orphanage
	// N = 1000, BPS ~ 10
	peerID := t.tangle.Peer.ID
	if peerID == 99 {
		type void struct{}
		oldTips := make(map[MessageID]void)
		t.WalkForOldestUnconfirmed(tipSet)

		for oldTip := range oldTips {
			tipSet.strongTips.Delete(oldTip)
		}
	}

	strongTips = tipSet.StrongTips(config.ParentsCount, t.tsa)
	// In the paper we consider all strong tips
	// weakTips = tipSet.WeakTips(config.ParentsCount-1, t.tsa)

	// Remove the weakTips-related codes
	// if len(weakTips) == 0 {
	// 	return
	// }

	// if strongParentsCount := len(strongTips); strongParentsCount < OptimalStrongParentsCount {
	// 	fillUpCount := config.ParentsCount - strongParentsCount

	// 	if fillUpCount >= len(weakTips) {
	// 		return
	// 	}

	// 	weakTips.Trim(fillUpCount)
	// 	return
	// }

	// if weakParentsCount := len(weakTips); weakParentsCount < OptimalWeakParentsCount {
	// 	fillUpCount := config.ParentsCount - weakParentsCount

	// 	if fillUpCount >= len(strongTips) {
	// 		return
	// 	}

	// 	strongTips.Trim(fillUpCount)
	// 	return
	// }

	// strongTips.Trim(OptimalStrongParentsCount)
	// weakTips.Trim(OptimalWeakParentsCount)

	return
}

func (t *TipManager) AddMsgProcessedCounter(color Color, amount uint64) {
	t.msgProcessedMutex.Lock()
	defer t.msgProcessedMutex.Unlock()

	t.msgProcessedCounter[color] += amount
}

func (t *TipManager) GetMsgProcessedCounter(color Color) uint64 {
	t.msgProcessedMutex.RLock()
	defer t.msgProcessedMutex.RUnlock()

	return t.msgProcessedCounter[color]
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

func (t *TipSet) Size() int {
	return t.strongTips.Size()
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
// RURTS: URTS with max parent age restriction
func (RURTS) TipSelect(tips *randommap.RandomMap, maxAmount int) []interface{} {

	var tipsNew []interface{}
	var tipsToReturn []interface{}
	amountLeft := maxAmount

	for {
		// Get amountLeft tips
		tipsNew = tips.RandomUniqueEntries(amountLeft)

		// If there are no tips, return the tipsToReturn
		if len(tipsNew) == 0 {
			break
		}

		// Get the current time
		currentTime := time.Now()
		for _, tip := range tipsNew {

			// If the time difference is greater than DeltaURTS, delete it from tips
			if currentTime.Sub(tip.(*Message).IssuanceTime).Seconds() > config.DeltaURTS {
				tips.Delete(tip)
			} else if fishingConditionFailed(tip) {
				tips.Delete(tip)
			} else {
				// Append the valid tip to tipsToReturn and decrease the amountLeft
				tipsToReturn = append(tipsToReturn, tip)
				amountLeft--
			}
		}

		// If maxAmount tips are appended to tipsToReturn already, return the tipsToReturn
		if amountLeft == 0 {
			break
		}
	}

	return tipsToReturn

}

// TODO: implement fishing condition
func fishingConditionFailed(tip interface{}) bool {
	// Traversing the parents of the tip
	// If the tip contains the block with `issuance time < Minimum Supported Time`
	//   return true
	// else return false
	return false
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
