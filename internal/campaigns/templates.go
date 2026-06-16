// Built-in campaign sequence templates.
package campaigns

// ThreeEmailOneLiTouch is the canonical B2B outbound sequence:
// 3 emails + 1 LinkedIn connect over 10 days.
var ThreeEmailOneLiTouch = Campaign{
	Name:       "3 Email + 1 LinkedIn Touch",
	TemplateID: "3_email_1_li",
	Status:     "draft",
	Steps: []Step{
		{ID: "email_1", Channel: "email", DayOffset: 0, Subject: "Quick question, {{first_name}}",
			Body: "Hi {{first_name}},\n\nSaw your work at {{company}} — {{signal}}.\n\nWorth a 15-min conversation?\n\nBest,\n{{sender_name}}"},
		{ID: "li_connect", Channel: "task", DayOffset: 3,
			Body: "Send LinkedIn connection request to {{first_name}} ({{linkedin_url}}). Note: 'Hi {{first_name}} — just emailed you about {{topic}}.'"},
		{ID: "email_2", Channel: "email", DayOffset: 5, Subject: "Re: Quick question, {{first_name}}",
			Body: "Hi {{first_name}},\n\nFollowing up on my note last week. Happy to share a 2-min teardown if useful.\n\nBest,\n{{sender_name}}"},
		{ID: "email_3", Channel: "email", DayOffset: 10, Subject: "Closing the loop, {{first_name}}",
			Body: "Hi {{first_name}},\n\nClosing the loop on this — happy to pick this back up whenever it's useful.\n\nBest,\n{{sender_name}}"},
	},
}

// FiveEmailCold is a longer 5-touch sequence over 21 days.
var FiveEmailCold = Campaign{
	Name:       "5 Email Cold",
	TemplateID: "5_email_cold",
	Status:     "draft",
	Steps: []Step{
		{ID: "email_1", Channel: "email", DayOffset: 0, Subject: "{{first_name}} — quick question",
			Body: "Hi {{first_name}}, saw {{signal}}. Worth a 15-min conversation?"},
		{ID: "email_2", Channel: "email", DayOffset: 3, Subject: "Re: {{first_name}} — quick question",
			Body: "Hi {{first_name}}, following up. Happy to share a teardown of the 3 main contenders."},
		{ID: "email_3", Channel: "email", DayOffset: 7, Subject: "{{first_name}} + {{company}}",
			Body: "Hi {{first_name}}, found a relevant case study on this. Want me to send it over?"},
		{ID: "email_4", Channel: "email", DayOffset: 14, Subject: "Different angle, {{first_name}}",
			Body: "Hi {{first_name}}, a different thought on this — what if {{alternative_approach}}?"},
		{ID: "email_5", Channel: "email", DayOffset: 21, Subject: "Closing the loop",
			Body: "Hi {{first_name}}, closing the loop on this. Happy to pick back up whenever useful."},
	},
}

// TwoLiOneEmailWarm is a LinkedIn-first warm touch.
var TwoLiOneEmailWarm = Campaign{
	Name:       "2 LinkedIn + 1 Email Warm",
	TemplateID: "2_li_1_email_warm",
	Status:     "draft",
	Steps: []Step{
		{ID: "li_connect", Channel: "task", DayOffset: 0,
			Body: "Send LinkedIn connection request. Note: 'Hi {{first_name}} — saw your work at {{company}}.'"},
		{ID: "li_message", Channel: "task", DayOffset: 2,
			Body: "Once connected, send: 'Hi {{first_name}} — {{signal}}. Worth a quick chat?'"},
		{ID: "email", Channel: "email", DayOffset: 5, Subject: "Following up, {{first_name}}",
			Body: "Hi {{first_name}}, following up on my LinkedIn message. Worth a 15-min conversation?"},
	},
}

// AvailableTemplates returns all built-in templates.
func AvailableTemplates() map[string]Campaign {
	return map[string]Campaign{
		"3_email_1_li":       ThreeEmailOneLiTouch,
		"5_email_cold":       FiveEmailCold,
		"2_li_1_email_warm":  TwoLiOneEmailWarm,
	}
}
