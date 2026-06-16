// Package sequencer — Built-in multi-channel sequence templates.
//
// Each template is a pre-built outreach sequence combining email, LinkedIn,
// Twitter/X, phone, and content steps. Use these as starting points or
// create custom sequences via the CLI or MCP.
package sequencer

// ── Multi-Channel Templates ────────────────────────────────────────────────────

// MultiFiveTouch is the full-spectrum 5-touch sequence:
// Email → LinkedIn connect → Email → Twitter engage → Email
var MultiFiveTouch = Sequence{
	Name:        "Multi-Channel 5-Touch",
	Description: "Email → LinkedIn connect → Follow-up email → X/Twitter engage → Breakup email",
	Status:      "draft",
	Steps: []Step{
		{
			ID: "email_1", Channel: "email", DayOffset: 0,
			Subject: "Quick question, {{first_name}}",
			Body: "Hi {{first_name}},\n\nSaw your work at {{company}} — {{signal}}.\n\nWorth a 15-min conversation?\n\nBest,\n{{sender_name}}",
		},
		{
			ID: "li_connect", Channel: "li_connect", DayOffset: 2,
			Body: "Hi {{first_name}} — just sent you an email about {{topic}}. Would love to connect.",
		},
		{
			ID: "email_2", Channel: "email", DayOffset: 5,
			Subject: "Re: Quick question, {{first_name}}",
			Body: "Hi {{first_name}},\n\nFollowing up on my note. Happy to share a 2-min teardown of how {{company}}'s peers are solving this.\n\nBest,\n{{sender_name}}",
		},
		{
			ID: "x_engage", Channel: "x_follow", DayOffset: 7,
			Body: "Follow @{{twitter_handle}} and engage with their recent content before reaching out.",
		},
		{
			ID: "email_3", Channel: "email", DayOffset: 10,
			Subject: "Closing the loop, {{first_name}}",
			Body: "Hi {{first_name}},\n\nClosing the loop on this — happy to pick this back up whenever it's useful.\n\nBest,\n{{sender_name}}",
		},
	},
}

// LinkedInFirst is LinkedIn-primary warm outreach:
// Connect → DM → Email follow-up
var LinkedInFirst = Sequence{
	Name:        "LinkedIn-First Warm",
	Description: "LinkedIn connect → DM → Email follow-up. Best for warm intros.",
	Status:      "draft",
	Steps: []Step{
		{
			ID: "li_connect", Channel: "li_connect", DayOffset: 0,
			Body: "Hi {{first_name}} — saw your work at {{company}} around {{topic}}. Would love to connect and share notes.",
		},
		{
			ID: "wait_connect", Channel: "wait", DayOffset: 2, WaitHours: 48,
			Body: "",
		},
		{
			ID: "li_message", Channel: "li_message", DayOffset: 3,
			Body: "Hi {{first_name}} — thanks for connecting! {{signal}} — worth a quick chat?",
		},
		{
			ID: "email", Channel: "email", DayOffset: 7,
			Subject: "Following up, {{first_name}}",
			Body: "Hi {{first_name}}, following up on my LinkedIn message. Worth a 15-min conversation?\n\nBest,\n{{sender_name}}",
		},
	},
}

// ContentLed builds authority before outreach:
// LinkedIn post → Comment on their post → Email referencing both
var ContentLed = Sequence{
	Name:        "Content-Led Authority",
	Description: "Post content → Comment on prospect's content → Email referencing both",
	Status:      "draft",
	Steps: []Step{
		{
			ID: "li_post", Channel: "li_post", DayOffset: 0,
			Body: "Publish a LinkedIn post about {{topic}} relevant to {{company}}'s space. Tag key themes {{first_name}} cares about.",
		},
		{
			ID: "li_comment", Channel: "li_comment", DayOffset: 2,
			Body: "Find and thoughtfully comment on a recent post by {{first_name}} ({{linkedin_url}}). Add genuine insight.",
		},
		{
			ID: "email", Channel: "email", DayOffset: 4,
			Subject: "Saw your post, {{first_name}}",
			Body: "Hi {{first_name}},\n\nLoved your recent take on {{topic}}. I just published something related — would value your perspective.\n\nAlso curious: how is {{company}} thinking about {{angle}}?\n\nWorth a quick chat?\n\nBest,\n{{sender_name}}",
		},
	},
}

// PhoneFirst is a phone-heavy sequence for high-value accounts:
// Email → Phone call → LinkedIn → SMS
var PhoneFirst = Sequence{
	Name:        "Phone-First High-Touch",
	Description: "Email → Phone call → LinkedIn connect → SMS follow-up. For Tier 1 accounts.",
	Status:      "draft",
	Steps: []Step{
		{
			ID: "email_1", Channel: "email", DayOffset: 0,
			Subject: "Before I call, {{first_name}}",
			Body: "Hi {{first_name}},\n\nI'll be reaching out this week about {{topic}}. Saw {{signal}} at {{company}}.\n\nBest,\n{{sender_name}}",
		},
		{
			ID: "phone_1", Channel: "phone", DayOffset: 2,
			Body: "Call {{first_name}} at {{phone}}. Talking points:\n- {{signal}}\n- How {{company}} handles {{topic}}\n- 15-min meeting request",
		},
		{
			ID: "li_connect", Channel: "li_connect", DayOffset: 4,
			Body: "Hi {{first_name}} — tried reaching you about {{topic}}. Would love to connect.",
		},
		{
			ID: "sms", Channel: "sms", DayOffset: 5,
			Body: "Hi {{first_name}}, this is {{sender_name}}. Sent you an email + tried calling about {{topic}}. Quick chat this week?",
		},
	},
}

// TwitterEngage warms up on Twitter/X before outreach:
// Follow → Like/Reply → DM → Email
var TwitterEngage = Sequence{
	Name:        "Twitter/X Engage → DM",
	Description: "Follow on X → Engage with content → DM → Email follow-up",
	Status:      "draft",
	Steps: []Step{
		{
			ID: "x_follow", Channel: "x_follow", DayOffset: 0,
			Body: "Follow @{{twitter_handle}} on X/Twitter.",
		},
		{
			ID: "x_engage", Channel: "task", DayOffset: 1,
			Body: "Like and reply to 1-2 recent tweets by @{{twitter_handle}}. Add genuine value.",
		},
		{
			ID: "x_dm", Channel: "x_dm", DayOffset: 3,
			Body: "Hey {{first_name}} — been following your takes on {{topic}}. We're working on something at {{sender_company}} that might be relevant. Mind if I share a quick overview?",
		},
		{
			ID: "email", Channel: "email", DayOffset: 6,
			Subject: "Following up, {{first_name}}",
			Body: "Hi {{first_name}},\n\nFollowing up on my DM on X. Saw {{signal}} — worth a 15-min conversation?\n\nBest,\n{{sender_name}}",
		},
	},
}

// FullCourtPress is the maximum-touch sequence across all channels:
// Email → LinkedIn → X/Twitter → Phone → SMS → Breakup email
var FullCourtPress = Sequence{
	Name:        "Full Court Press",
	Description: "6-touch sequence across Email, LinkedIn, X, Phone, and SMS. Maximum penetration.",
	Status:      "draft",
	Steps: []Step{
		{
			ID: "email_1", Channel: "email", DayOffset: 0,
			Subject: "{{first_name}} — quick question",
			Body: "Hi {{first_name}},\n\nSaw {{signal}} at {{company}}. Worth a 15-min conversation?\n\nBest,\n{{sender_name}}",
		},
		{
			ID: "li_connect", Channel: "li_connect", DayOffset: 2,
			Body: "Hi {{first_name}} — just emailed you about {{topic}}. Would love to connect.",
		},
		{
			ID: "x_follow", Channel: "x_follow", DayOffset: 3,
			Body: "Follow @{{twitter_handle}} and engage with recent content.",
		},
		{
			ID: "email_2", Channel: "email", DayOffset: 5,
			Subject: "Re: {{first_name}} — quick question",
			Body: "Hi {{first_name}}, following up. Happy to share how {{company}}'s peers are solving this.",
		},
		{
			ID: "phone", Channel: "phone", DayOffset: 7,
			Body: "Call {{first_name}} at {{phone}}. Reference previous emails + LinkedIn connection.",
		},
		{
			ID: "sms", Channel: "sms", DayOffset: 8,
			Body: "Hi {{first_name}}, this is {{sender_name}}. Tried reaching you about {{topic}}. Quick chat this week?",
		},
		{
			ID: "email_3", Channel: "email", DayOffset: 12,
			Subject: "Closing the loop",
			Body: "Hi {{first_name}},\n\nClosing the loop — happy to reconnect whenever this becomes a priority.\n\nBest,\n{{sender_name}}",
		},
	},
}

// AvailableTemplates returns all built-in sequence templates.
func AvailableTemplates() map[string]Sequence {
	return map[string]Sequence{
		"multi_5touch":    MultiFiveTouch,
		"li_first":        LinkedInFirst,
		"content_led":     ContentLed,
		"phone_first":     PhoneFirst,
		"twitter_engage":  TwitterEngage,
		"full_court":      FullCourtPress,
		// Keep backward compat with campaign templates
		"3_email_1_li":    ThreeEmailOneLiSequence,
		"5_email_cold":    FiveEmailColdSequence,
	}
}

// ThreeEmailOneLiSequence is the classic sequence migrated to the new format.
var ThreeEmailOneLiSequence = Sequence{
	Name:   "3 Email + 1 LinkedIn Touch",
	Status: "draft",
	Steps: []Step{
		{ID: "email_1", Channel: "email", DayOffset: 0, Subject: "Quick question, {{first_name}}",
			Body: "Hi {{first_name}},\n\nSaw your work at {{company}} — {{signal}}.\n\nWorth a 15-min conversation?\n\nBest,\n{{sender_name}}"},
		{ID: "li_connect", Channel: "li_connect", DayOffset: 3,
			Body: "Send LinkedIn connection request. Note: 'Hi {{first_name}} — just emailed you about {{topic}}.'"},
		{ID: "email_2", Channel: "email", DayOffset: 5, Subject: "Re: Quick question, {{first_name}}",
			Body: "Hi {{first_name}},\n\nFollowing up on my note last week. Happy to share a teardown if useful.\n\nBest,\n{{sender_name}}"},
		{ID: "email_3", Channel: "email", DayOffset: 10, Subject: "Closing the loop, {{first_name}}",
			Body: "Hi {{first_name}},\n\nClosing the loop — happy to pick this back up whenever useful.\n\nBest,\n{{sender_name}}"},
	},
}

// FiveEmailColdSequence is a 5-email cold sequence migrated to new format.
var FiveEmailColdSequence = Sequence{
	Name:   "5 Email Cold",
	Status: "draft",
	Steps: []Step{
		{ID: "email_1", Channel: "email", DayOffset: 0, Subject: "{{first_name}} — quick question",
			Body: "Hi {{first_name}}, saw {{signal}}. Worth a 15-min conversation?"},
		{ID: "email_2", Channel: "email", DayOffset: 3, Subject: "Re: {{first_name}} — quick question",
			Body: "Hi {{first_name}}, following up. Happy to share a teardown of the 3 main contenders."},
		{ID: "email_3", Channel: "email", DayOffset: 7, Subject: "{{first_name}} + {{company}}",
			Body: "Hi {{first_name}}, found a relevant case study. Want me to send it over?"},
		{ID: "email_4", Channel: "email", DayOffset: 14, Subject: "Different angle, {{first_name}}",
			Body: "Hi {{first_name}}, a different thought — what if {{alternative_approach}}?"},
		{ID: "email_5", Channel: "email", DayOffset: 21, Subject: "Closing the loop",
			Body: "Hi {{first_name}}, closing the loop on this. Happy to pick back up whenever useful."},
	},
}
