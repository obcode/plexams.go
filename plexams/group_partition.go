package plexams

import (
	"context"
	"fmt"
	"sort"

	"github.com/obcode/plexams.go/graph/model"
	"github.com/rs/zerolog/log"
)

func (p *Plexams) PartitionGroups() error {
	ctx := context.Background()
	examGroups, err := p.ExamGroups(ctx)
	if err != nil {
		log.Error().Err(err).Msg("cannot get exam groups")
	}

	partitions := p.partitionGroups(examGroups)

	for _, partition := range partitions {
		fmt.Println(partition)
	}

	return nil
}

func (p *Plexams) partitionGroups(groups []*model.ExamGroup) [][]int {
	adjList := make(map[int][]int)
	inPartition := make(map[int]bool)

	for _, group := range groups {
		conflictList := make([]int, 0)
		for _, conflict := range group.ExamGroupInfo.Conflicts {
			conflictList = append(conflictList, conflict.ExamGroupCode)
		}
		adjList[group.ExamGroupCode] = conflictList
	}

	for k, v := range adjList {
		fmt.Printf("%d -> %v\n", k, v)
	}

	partitions := make([][]int, 0)

	for _, group := range groups {
		if !inPartition[group.ExamGroupCode] {
			partition := []int{group.ExamGroupCode}
			for {
				partitionBefore := partition
				for _, source := range partition {
					fmt.Printf("Adding: %d -> %v\n", source, adjList[source])

					partition = append(partition, adjList[source]...)
				}

				partition = removeDuplicates(partition)
				sort.Ints(partition)

				fmt.Printf("got: %v\n", partition)

				if len(partitionBefore) == len(partition) {
					break
				}
			}
			partitions = append(partitions, partition)

			for _, code := range partition {
				inPartition[code] = true
			}
		}
	}

	return partitions
}
