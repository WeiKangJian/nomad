package scheduler

import (
	"fmt"

	"math"

	"github.com/hashicorp/nomad/nomad/structs"
)

// deviceAllocator is used to allocate devices to allocations. The allocator
// tracks availability as to not double allocate devices.
type deviceAllocator struct {
	*structs.DeviceAccounter

	ctx Context
}

// newDeviceAllocator returns a new device allocator. The node is used to
// populate the set of available devices based on what healthy device instances
// exist on the node.
func newDeviceAllocator(ctx Context, n *structs.Node) *deviceAllocator {
	return &deviceAllocator{
		ctx:             ctx,
		DeviceAccounter: structs.NewDeviceAccounter(n),
	}
}

// AssignDevice takes a device request and returns an assignment as well as a
// score for the assignment. If no assignment could be made, an error is
// returned explaining why.
func (d *deviceAllocator) AssignDevice(ask *structs.RequestedDevice) (out *structs.AllocatedDeviceResource, score float64, err error) {
	// Try to hot path
	if len(d.Devices) == 0 {
		return nil, 0.0, fmt.Errorf("no devices available")
	}
	if ask.Count == 0 {
		return nil, 0.0, fmt.Errorf("invalid request of zero devices")
	}

	// Hold the current best offer
	var offer *structs.AllocatedDeviceResource
	var offerScore float64
	var matchedWeights float64

	// Determine the devices that are feasible based on availability and
	// constraints
	for id, devInst := range d.Devices {

		// Check if we have enough unused instances to use this
		//assignable := uint64(0)
		//for _, v := range devInst.Instances {
		//	if v == 0 {
		//		assignable++
		//	}
		//}

		// This device doesn't have enough instances
		// 当前Nomad不支持GPU上任务的混布，暂时去掉这一行
		//if assignable < ask.Count {
		//	continue
		//}

		// Check if the device works
		if !nodeDeviceMatches(d.ctx, devInst.Device, ask) {
			continue
		}

		// Score the choice
		var choiceScore float64

		// Track the sum of matched affinity weights in a separate variable
		// We return this if this device had the best score compared to other devices considered
		var sumMatchedWeights float64
		if l := len(ask.Affinities); l != 0 {
			totalWeight := 0.0
			for _, a := range ask.Affinities {
				// Resolve the targets
				lVal, lOk := resolveDeviceTarget(a.LTarget, devInst.Device)
				rVal, rOk := resolveDeviceTarget(a.RTarget, devInst.Device)

				totalWeight += math.Abs(float64(a.Weight))

				// Check if satisfied
				if !checkAttributeAffinity(d.ctx, a.Operand, lVal, rVal, lOk, rOk) {
					continue
				}
				choiceScore += float64(a.Weight)
				sumMatchedWeights += float64(a.Weight)
			}

			// normalize
			choiceScore /= totalWeight
		}

		// Only use the device if it is a higher score than we have already seen
		if offer != nil && choiceScore < offerScore {
			continue
		}

		// Set the new highest score
		offerScore = choiceScore

		// Set the new sum of matching affinity weights
		matchedWeights = sumMatchedWeights

		// Build the choice
		offer = &structs.AllocatedDeviceResource{
			Vendor:    id.Vendor,
			Type:      id.Type,
			Name:      id.Name,
			DeviceIDs: make([]string, 0, ask.Count),
		}

		assigned := uint64(0)
		for ; assigned < ask.Count; assigned++ {
			curValue := math.MaxInt
			var curId string
			for id, v := range devInst.Instances {
				if v <= curValue {
					curValue = v
					curId = id
				}
			}
			offer.DeviceIDs = append(offer.DeviceIDs, curId)
		}
		//for id, v := range devInst.Instances {
		//
		//	if assigned < ask.Count {
		//		assigned++
		//		offer.DeviceIDs = append(offer.DeviceIDs, id)
		//		if assigned == ask.Count {
		//			break
		//		}
		//	}
		//}
	}
	// Failed to find a match
	if offer == nil {
		return nil, 0.0, fmt.Errorf("no devices match request")
	}

	return offer, matchedWeights, nil
}

func (d *deviceAllocator) AssignGpu(ask *structs.RequestedDevice, allocs []*structs.Allocation) (
	out *structs.AllocatedDeviceResource, score float64, err error) {

	if len(d.Devices) == 0 {
		return nil, 0.0, fmt.Errorf("no devices available")
	}
	if ask.Count == 0 {
		return nil, 0.0, fmt.Errorf("invalid request of zero devices")
	}

	var offer *structs.AllocatedDeviceResource
	var matchedWeights float64
	var nodeGpuResource *structs.DeviceAccounterInstance

	for _, gpuResource := range d.Devices {
		if gpuResource.Device.ID().Matches(ask.ID()) {
			nodeGpuResource = gpuResource
			break
		}
	}

	if nodeGpuResource == nil {
		return nil, 0.0, fmt.Errorf("no gpu devices match request")
	}

	// Determine the devices that are feasible based on availability and
	// constraints
	for id, devInst := range d.Devices {
		if devInst.Device != nil && devInst.Device.ID().Matches(ask.ID()) {
			curGpuUsageMap := d.GetCurGpuUsage(ask.ID(), allocs, nodeGpuResource)
			if len(curGpuUsageMap) == 0 {
				continue
			}
			var resID string
			var minUsage int64 = math.MaxInt64
			// 选instance里当前使用少的那一个，不在这里进行检查，检查逻辑在rank.go里
			for id, usage := range curGpuUsageMap {
				if usage < minUsage {
					resID = id
					minUsage = usage
				}
			}

			if len(resID) == 0 {
				return nil, 0.0, fmt.Errorf("get usage of gpu error")
			}

			// 选中就发offer
			offer = &structs.AllocatedDeviceResource{
				Vendor:    id.Vendor,
				Type:      id.Type,
				Name:      id.Name,
				DeviceIDs: make([]string, 0, ask.Count),
			}
			assigned := uint64(0)
			// 暂时不支持job里的count>1,如果>1那么都发同一个deviceID
			for ; assigned < ask.Count; assigned++ {
				offer.DeviceIDs = append(offer.DeviceIDs, resID)
			}
			break
		}
	}

	// Failed to find a match
	if offer == nil {
		return nil, 0.0, fmt.Errorf("no devices match request")
	}

	return offer, matchedWeights, nil
}
