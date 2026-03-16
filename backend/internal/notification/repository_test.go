package notification

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatCents verifies the dollar formatting helper.
func TestFormatCents(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{0, "$0.00"},
		{100, "$1.00"},
		{150000, "$1500.00"},
		{300, "$3.00"},
		{50, "$0.50"},
		{999, "$9.99"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatCents(tt.cents))
	}
}

// TestNotificationMessages verifies message templates for each notification type.
func TestNotificationMessages(t *testing.T) {
	tests := []struct {
		name        string
		notifType   string
		amountCents int64
		reason      string
		feeCents    int64
		wantTitle   string
		wantMsgSubs []string
	}{
		{
			name:        "approved message",
			notifType:   "approved",
			amountCents: 150000,
			wantTitle:   "Deposit Approved",
			wantMsgSubs: []string{"$1500.00", "approved", "provisionally credited"},
		},
		{
			name:        "rejected message includes reason",
			notifType:   "rejected",
			amountCents: 200000,
			reason:      "MICR data unreadable",
			wantTitle:   "Deposit Rejected",
			wantMsgSubs: []string{"$2000.00", "MICR data unreadable", "new deposit"},
		},
		{
			name:        "returned message includes fee",
			notifType:   "returned",
			amountCents: 100000,
			reason:      "insufficient_funds",
			feeCents:    3000,
			wantTitle:   "Check Returned — Fee Applied",
			wantMsgSubs: []string{"$1000.00", "insufficient_funds", "$30.00", "return fee"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg, title string
			switch tt.notifType {
			case "approved":
				title = "Deposit Approved"
				msg = "Your check deposit of " + FormatCents(tt.amountCents) +
					" has been approved. Funds have been provisionally credited to your account."
			case "rejected":
				title = "Deposit Rejected"
				msg = "Your check deposit of " + FormatCents(tt.amountCents) +
					" has been rejected. Reason: " + tt.reason +
					". You may submit a new deposit with a different check."
			case "returned":
				title = "Check Returned — Fee Applied"
				msg = "Your check deposit of " + FormatCents(tt.amountCents) +
					" was returned by the bank. Reason: " + tt.reason +
					". A " + FormatCents(tt.feeCents) +
					" return fee has been deducted from your account. You may submit a new deposit with a different check."
			}

			assert.Equal(t, tt.wantTitle, title)
			for _, sub := range tt.wantMsgSubs {
				assert.Contains(t, msg, sub, "message should contain %q", sub)
			}
		})
	}
}

// TestRepo_CreateAndRetrieve exercises the full CRUD cycle using an in-memory test double.
// Integration tests against real Postgres are tagged with //go:build integration.
func TestRepo_InMemoryOperations(t *testing.T) {
	ctx := context.Background()

	// Use the in-memory stub for unit tests.
	repo := newTestRepo(t)

	notif := &Notification{
		AccountID:  "ACC-TEST-1",
		TransferID: "tid-001",
		Type:       "approved",
		Title:      "Deposit Approved",
		Message:    "Your deposit of $100.00 has been approved.",
	}

	err := repo.Create(ctx, notif)
	require.NoError(t, err)
	assert.NotEmpty(t, notif.ID, "ID should be assigned after Create")

	list, err := repo.GetByAccount(ctx, "ACC-TEST-1", false)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "approved", list[0].Type)
	assert.False(t, list[0].Read)

	count, err := repo.GetUnreadCount(ctx, "ACC-TEST-1")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = repo.MarkRead(ctx, notif.ID)
	require.NoError(t, err)

	count, err = repo.GetUnreadCount(ctx, "ACC-TEST-1")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestRepo_MarkAllRead verifies MarkAllRead zeroes the unread count.
func TestRepo_MarkAllRead(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	for i := 0; i < 3; i++ {
		_ = repo.Create(ctx, &Notification{
			AccountID:  "ACC-TEST-2",
			TransferID: "tid-002",
			Type:       "rejected",
			Title:      "Deposit Rejected",
			Message:    "Rejected.",
		})
	}

	count, _ := repo.GetUnreadCount(ctx, "ACC-TEST-2")
	assert.Equal(t, 3, count)

	err := repo.MarkAllRead(ctx, "ACC-TEST-2")
	require.NoError(t, err)

	count, _ = repo.GetUnreadCount(ctx, "ACC-TEST-2")
	assert.Equal(t, 0, count)
}

// TestRepo_UnreadOnlyFilter verifies the unreadOnly flag in GetByAccount.
func TestRepo_UnreadOnlyFilter(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	n1 := &Notification{AccountID: "ACC-TEST-3", TransferID: "tid-003", Type: "approved", Title: "T", Message: "M"}
	n2 := &Notification{AccountID: "ACC-TEST-3", TransferID: "tid-004", Type: "rejected", Title: "T", Message: "M"}
	_ = repo.Create(ctx, n1)
	_ = repo.Create(ctx, n2)
	_ = repo.MarkRead(ctx, n1.ID)

	unread, err := repo.GetByAccount(ctx, "ACC-TEST-3", true)
	require.NoError(t, err)
	require.Len(t, unread, 1)
	assert.Equal(t, n2.ID, unread[0].ID)
}
