package network

import (
	"strconv"
	"time"

	"github.com/iotaledger/hive.go/crypto"
	"github.com/iotaledger/hive.go/datastructure/set"
	"github.com/iotaledger/multivers-simulation/config"
)

// region AdversaryType ////////////////////////////////////////////////////////////////////////////////////////////////

type AdversaryType int

const (
	HonestNode AdversaryType = iota
	ShiftOpinion
	TheSameOpinion
	NoGossip
	Blowball
)

func ToAdversaryType(adv int) AdversaryType {
	switch adv {
	case int(ShiftOpinion):
		return ShiftOpinion
	case int(TheSameOpinion):
		return TheSameOpinion
	case int(NoGossip):
		return NoGossip
	case int(Blowball):
		return Blowball
	}
	return HonestNode
}

func AdversaryTypeToString(adv AdversaryType) string {
	switch adv {
	case HonestNode:
		return "Honest"
	case ShiftOpinion:
		return "ShiftingOpinion"
	case TheSameOpinion:
		return "TheSameOpinion"
	case NoGossip:
		return "NoGossip"
	case Blowball:
		return "Blowball"
	}
	return ""
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region AdversaryGroup ////////////////////////////////////////////////////////////////////////////////////////////////

var AdversaryNodeIDToGroupIDMap = make(map[int]int)

func IsAdversary(nodeID int) bool {
	_, ok := AdversaryNodeIDToGroupIDMap[nodeID]
	return ok
}

type AdversaryGroup struct {
	NodeIDs              []int
	GroupMana            float64
	TargetManaPercentage float64
	Delay                time.Duration
	AdversaryType        AdversaryType
	InitColor            string
	NodeCount            int
}

func (g *AdversaryGroup) AddNodeID(id, groupId int) {
	g.NodeIDs = append(g.NodeIDs, id)
	AdversaryNodeIDToGroupIDMap[id] = groupId
}

type AdversaryGroups []*AdversaryGroup

func NewAdversaryGroups() (groups AdversaryGroups) {
	groups = make(AdversaryGroups, 0, len(config.Params.AdversaryTypes))
	for i, configAdvType := range config.Params.AdversaryTypes {
		targetMana := float64(1)
		delay := config.Params.MinDelay
		color := ""
		nCount := 1

		if len(config.Params.AdversaryMana) > 0 {
			targetMana = config.Params.AdversaryMana[i]
		}

		if len(config.Params.AdversaryDelays) > 0 {
			delay = config.Params.AdversaryDelays[i]
		}

		if len(config.Params.AdversaryNodeCounts) > 0 {
			nCount = config.Params.AdversaryNodeCounts[i]
		}

		color = config.Params.AdversaryInitColors[i]
		group := &AdversaryGroup{
			NodeIDs:              make([]int, 0, nCount),
			TargetManaPercentage: targetMana,
			Delay:                time.Millisecond * time.Duration(delay),
			AdversaryType:        ToAdversaryType(configAdvType),
			InitColor:            color,
			NodeCount:            nCount,
		}
		groups = append(groups, group)
	}

	return
}

// CalculateWeightTotalConfig returns how many nodes will be used for weight distribution and their total weight
// after excluding all adversary nodes that will not be selected randomly
func (g *AdversaryGroups) CalculateWeightTotalConfig() (int, float64) {
	totalAdv := 0
	totalAdvManaPercentage := float64(0)

	for _, group := range *g {
		totalAdv += group.NodeCount
		totalAdvManaPercentage += group.TargetManaPercentage
	}
	totalCount := config.Params.NodesCount - totalAdv
	totalWeight := float64(config.Params.NodesTotalWeight) * (1 - totalAdvManaPercentage/100)
	return totalCount, totalWeight
}

// UpdateAdversaryNodes assigns adversary nodes in AdversaryGroups to correct nodeIDs and updates their mana
func (g *AdversaryGroups) UpdateAdversaryNodes(weightDistribution []uint64) []uint64 {
	g.updateGroupMana()

	// Adversary nodes are taking indexes from the end, excluded randomly chosen nodes
	advIndex := len(weightDistribution)
	// weight distribution with adversary weights appended at the ned
	newWeights := g.updateAdvIDAndWeights(advIndex, weightDistribution)

	return newWeights
}

func (g *AdversaryGroups) updateAdvIDAndWeights(advIndex int, newWeights []uint64) []uint64 {
	for groupIndex, group := range *g {
		for i := 0; i < group.NodeCount; i++ {
			group.AddNodeID(advIndex, groupIndex)
			advIndex++
			// append adversary weight at the end of weight distribution
			nodeWeight := uint64(group.GroupMana / float64(group.NodeCount))
			newWeights = append(newWeights, nodeWeight)
		}
	}
	return newWeights
}

func (g *AdversaryGroups) updateGroupMana() {
	for _, group := range *g {
		group.GroupMana = group.TargetManaPercentage * float64(config.Params.NodesTotalWeight) / 100.0
	}
}

func (g *AdversaryGroups) ApplyNetworkDelayForAdversaryNodes(network *Network) {
	for _, adversaryGroup := range *g {
		for _, nodeID := range adversaryGroup.NodeIDs {
			peer := network.Peer(nodeID)
			for _, neighbor := range peer.Neighbors {
				neighbor.SetDelay(adversaryGroup.Delay)
			}
		}
	}
}

func (g *AdversaryGroups) ApplyNeighborsAdversaryNodes(network *Network, configuration *Configuration) {
	for _, adversaryGroup := range *g {
		for _, nodeID := range adversaryGroup.NodeIDs {
			adversary := network.Peer(nodeID)
			for _, peer := range network.Peers {
				adversary.Neighbors[peer.ID] = NewConnection(
					network.Peers[peer.ID].Socket,
					adversaryGroup.Delay,
					0,
					configuration,
				)
			}
		}
	}
}

func randomWeightIndex(weights []uint64, count int) (randomWeights []int) {
	selectedPeers := set.New()
	for len(randomWeights) < count {
		if randomIndex := crypto.Randomness.Intn(len(weights)); selectedPeers.Add(randomIndex) {
			randomWeights = append(randomWeights, randomIndex)
		}
	}
	return
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Accidental ///////////////////////////////////////////////////////////////////////////////////////////////////

func GetAccidentalIssuers(network *Network) []*Peer {
	peers := make([]*Peer, 0)
	randomCount := 0
	for i := 0; i < len(config.Params.AccidentalMana); i++ {
		switch config.Params.AccidentalMana[i] {
		case "max":
			peers = append(peers, network.Peer(0))
		case "min":
			peers = append(peers, network.Peer(len(network.WeightDistribution.weights)-1))
		case "random":
			randomCount++
		default:
			customId, err := strconv.Atoi(config.Params.AccidentalMana[i])
			if err != nil || config.Params.NodesCount-1 < customId || customId < 0 {
				log.Warnf("AccidentalMana parameter: %s is incorrect, so not processed", config.Params.AccidentalMana[i])
			} else {
				peers = append(peers, network.Peer(customId))
			}
		}
	}
	if randomCount > 0 {
		for _, selectedNode := range network.RandomPeers(randomCount) {
			peers = append(peers, selectedNode)
		}
	}
	return peers
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
