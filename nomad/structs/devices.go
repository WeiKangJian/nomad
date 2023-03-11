package structs

import (
	"strconv"
	"strings"
)

// DeviceAccounter is used to account for device usage on a node. It can detect
// when a node is oversubscribed and can be used for deciding what devices are
// free
type DeviceAccounter struct {
	// Devices maps a device group to its device accounter instance
	Devices map[DeviceIdTuple]*DeviceAccounterInstance
}

// DeviceAccounterInstance wraps a device and adds tracking to the instances of
// the device to determine if they are free or not.
type DeviceAccounterInstance struct {
	// Device is the device being wrapped
	Device *NodeDeviceResource

	// Instances is a mapping of the device IDs to their usage.
	// Only a value of 0 indicates that the instance is unused.
	Instances map[string]int
}

// NewDeviceAccounter returns a new device accounter. The node is used to
// populate the set of available devices based on what healthy device instances
// exist on the node.
func NewDeviceAccounter(n *Node) *DeviceAccounter {
	numDevices := 0
	var devices []*NodeDeviceResource

	// COMPAT(0.11): Remove in 0.11
	if n.NodeResources != nil {
		numDevices = len(n.NodeResources.Devices)
		devices = n.NodeResources.Devices
	}

	d := &DeviceAccounter{
		Devices: make(map[DeviceIdTuple]*DeviceAccounterInstance, numDevices),
	}

	for _, dev := range devices {
		id := *dev.ID()
		d.Devices[id] = &DeviceAccounterInstance{
			Device:    dev,
			Instances: make(map[string]int, len(dev.Instances)),
		}
		for _, instance := range dev.Instances {
			// Skip unhealthy devices as they aren't allocatable
			if !instance.Healthy {
				continue
			}

			d.Devices[id].Instances[instance.ID] = 0
		}
	}

	return d
}

// AddAllocs takes a set of allocations and internally marks which devices are
// used. If a device is used more than once by the set of passed allocations,
// the collision will be returned as true.
func (d *DeviceAccounter) AddAllocs(allocs []*Allocation) (collision bool) {
	for _, a := range allocs {
		// Filter any terminal allocation
		if a.ClientTerminalStatus() {
			continue
		}

		// COMPAT(0.11): Remove in 0.11
		// If the alloc doesn't have the new style resources, it can't have
		// devices
		if a.AllocatedResources == nil {
			continue
		}

		// Go through each task  resource
		for _, tr := range a.AllocatedResources.Tasks {

			// Go through each assigned device group
			for _, device := range tr.Devices {
				devID := device.ID()

				// Go through each assigned device
				for _, instanceID := range device.DeviceIDs {

					// Mark that we are using the device. It may not be in the
					// map if the device is no longer being fingerprinted, is
					// unhealthy, etc.
					if devInst, ok := d.Devices[*devID]; ok {
						if i, ok := devInst.Instances[instanceID]; ok {
							// Mark that the device is in use
							devInst.Instances[instanceID]++

							if i != 0 {
								// 这里会做一个检查，需要去掉，方便gpu混部
								collision = false
							}
						}
					}
				}
			}
		}
	}

	return
}

// CheckGpuResource check if node gpu resource and allocations resource
func (d *DeviceAccounter) CheckGpuResource(deviceID *DeviceIdTuple, allocs []*Allocation, nodeGpuResource *DeviceAccounterInstance) bool {
	if nodeGpuResource == nil || nodeGpuResource.Device == nil || nodeGpuResource.Device.Attributes == nil {
		return false
	}
	gpuMemoryMap := d.GetCurGpuUsage(deviceID, allocs, nodeGpuResource)
	nodeInstanceGpuMemoryAttribute, ok := nodeGpuResource.Device.Attributes["memory"]
	if !ok || nodeInstanceGpuMemoryAttribute == nil {
		return false
	}
	var nodeInstanceGpuMemory int64
	if value, ok := nodeInstanceGpuMemoryAttribute.GetInt(); ok {
		if nodeInstanceGpuMemoryAttribute.Unit == "MiB" {
			nodeInstanceGpuMemory = value
		} else if nodeInstanceGpuMemoryAttribute.Unit == "GiB" {
			nodeInstanceGpuMemory = value * 1000
		}
	} else if value, ok := nodeInstanceGpuMemoryAttribute.GetFloat(); ok {
		if nodeInstanceGpuMemoryAttribute.Unit == "MiB" {
			nodeInstanceGpuMemory = int64(value)
		} else if nodeInstanceGpuMemoryAttribute.Unit == "GiB" {
			nodeInstanceGpuMemory = int64(value * 1000)
		}
	} else {
		return false
	}

	for gpuInstanceID := range nodeGpuResource.Instances {
		if gpuMemoryMap[gpuInstanceID] > nodeInstanceGpuMemory {
			return false
		}
	}

	return true
}

func (d *DeviceAccounter) GetCurGpuUsage(deviceID *DeviceIdTuple, allocs []*Allocation, nodeGpuResource *DeviceAccounterInstance) map[string]int64 {

	gpuMemoryMap := make(map[string]int64)
	if nodeGpuResource == nil || nodeGpuResource.Instances == nil {
		return gpuMemoryMap
	}
	for gpuInstanceID := range nodeGpuResource.Instances {
		gpuMemoryMap[gpuInstanceID] = 0
	}
	for _, a := range allocs {
		if a.AllocatedResources == nil {
			continue
		}

		// Go through each task  resource
		for taskName, allocationTaskResource := range a.AllocatedResources.Tasks {
			if allocationTaskResource == nil || allocationTaskResource.Devices == nil {
				continue
			}
			// Go through each assigned device group
			for _, device := range allocationTaskResource.Devices {
				if device.ID().Matches(deviceID) {
					// 拿到了被分配的显卡的instanceID
					for _, instanceID := range device.DeviceIDs {
						if curMemory, ok := gpuMemoryMap[instanceID]; ok {
							//拿到这个task要求的显存
							if a.TaskResources == nil {
								continue
							}
							resource, ok := a.TaskResources[taskName]
							if !ok || resource.Devices == nil || len(resource.Devices) == 0 {
								continue
							}
							for _, resourceDevice := range resource.Devices {
								if resourceDevice.Constraints == nil || len(resourceDevice.Constraints) == 0 {
									continue
								}
								for _, constrain := range resourceDevice.Constraints {
									if constrain != nil && constrain.LTarget == "${device.attr.memory}" {
										if constrain.Operand != ">" && constrain.Operand != ">=" {
											continue
										}
										requestResource := strings.Fields(strings.TrimSpace(constrain.RTarget))
										if len(requestResource) == 2 {
											if resource, err := strconv.ParseFloat(requestResource[0], 64); err == nil {
												if requestResource[1] == "MiB" {
													gpuMemoryMap[instanceID] = curMemory + int64(resource)
												} else if requestResource[1] == "GiB" {
													gpuMemoryMap[instanceID] = curMemory + int64(resource*1000)
												}
											}
										}
										break
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return gpuMemoryMap
}

// AddReserved marks the device instances in the passed device reservation as
// used and returns if there is a collision.
func (d *DeviceAccounter) AddReserved(res *AllocatedDeviceResource) (collision bool) {
	// Lookup the device.
	devInst, ok := d.Devices[*res.ID()]
	if !ok {
		return false
	}

	// For each reserved instance, mark it as used
	for _, id := range res.DeviceIDs {
		cur, ok := devInst.Instances[id]
		if !ok {
			continue
		}

		// It has already been used, so mark that there is a collision
		if cur != 0 {
			collision = true
		}
		devInst.Instances[id]++
	}

	return
}

// FreeCount returns the number of free device instances
func (i *DeviceAccounterInstance) FreeCount() int {
	count := 0
	for _, c := range i.Instances {
		if c == 0 {
			count++
		}
	}
	return count
}
