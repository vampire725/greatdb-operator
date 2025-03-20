package configmap

import (
	v1 "greatdb.com/greatdb-operator/api/v1"
)

func GetNextIndex(memberList []v1.MemberCondition) int {
	index := -1

	for _, member := range memberList {
		if member.Index > index {
			index = member.Index
		}
	}
	return index + 1
}
