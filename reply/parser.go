package reply

import (
	"strconv"
	"strings"
)

type ReplyCommand struct {
	Action  string
	Numbers []int
}

func ParseReply(text string) *ReplyCommand {
	text = strings.TrimSpace(strings.ToLower(text))

	if strings.HasPrefix(text, "done:") {
		nums := parseNumbers(strings.TrimPrefix(text, "done:"))
		if len(nums) > 0 {
			return &ReplyCommand{Action: "done", Numbers: nums}
		}
	}
	if strings.HasPrefix(text, "todo:") {
		nums := parseNumbers(strings.TrimPrefix(text, "todo:"))
		if len(nums) > 0 {
			return &ReplyCommand{Action: "todo", Numbers: nums}
		}
	}
	return nil
}

func parseNumbers(s string) []int {
	var nums []int
	for _, part := range strings.Split(strings.TrimSpace(s), ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && n > 0 {
			nums = append(nums, n)
		}
	}
	return nums
}
