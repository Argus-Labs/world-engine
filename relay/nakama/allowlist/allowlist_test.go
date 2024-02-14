package allowlist

import (
	"fmt"
	"testing"

	"pkg.world.dev/world-engine/assert"

	"pkg.world.dev/world-engine/relay/nakama/signer"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

func TestConcurrentKeyClaims(t *testing.T) {
	originalEnabled := Enabled
	Enabled = true
	t.Cleanup(func() {
		Enabled = originalEnabled
	})
	const (
		numOfUsers    = 10
		numOfBetaKeys = 10
	)
	fakeNK := testutils.NewFakeNakamaModule()
	ctx := testutils.CtxWithUserID(signer.AdminAccountID)
	resp, err := GenerateBetaKeys(ctx, fakeNK, GenKeysMsg{numOfBetaKeys})
	assert.NilError(t, err)

	var users []string
	for i := 0; i < numOfUsers; i++ {
		users = append(users, fmt.Sprintf("user-%d", i))
	}

	waitCh := make(chan struct{})
	type result struct {
		user        string
		key         string
		claimResult ClaimKeyRes
		err         error
	}
	allResults := make(chan result)
	for _, user := range users {
		for _, key := range resp.Keys {
			user := user
			key := key
			// Have every user attempt to claim every beta key. Each beta key should be used exactly 1 time and
			// each user should have 9 failed attempts and 1 successful attempt.
			go func() {
				userCtx := testutils.CtxWithUserID(user)
				// Block until the waitCh channel in closed to give us the best chance of running this section
				// of code concurrently.
				<-waitCh
				// Just send the results of these ClaimKey calls to the main test thread; the assertions will take place
				// there.
				r, err := ClaimKey(userCtx, fakeNK, ClaimKeyMsg{
					Key: key,
				})
				allResults <- result{
					user:        user,
					key:         key,
					claimResult: r,
					err:         err,
				}
			}()
		}
	}

	// Closing this channel unblocks all the goroutines above.
	close(waitCh)

	userFailureCount := map[string]int{}
	userSuccessCount := map[string]int{}
	usedKeyCount := map[string]int{}
	// Count how many successes there are for each user, how many failures, and which beta keys were actually used.
	for i := 0; i < numOfBetaKeys*numOfUsers; i++ {
		curr := <-allResults
		if curr.err != nil || !curr.claimResult.Success {
			userFailureCount[curr.user]++
		} else {
			userSuccessCount[curr.user]++
			usedKeyCount[curr.key]++
		}
	}

	assert.Equal(t, numOfUsers, len(userFailureCount))
	assert.Equal(t, numOfUsers, len(userSuccessCount))
	assert.Equal(t, numOfBetaKeys, len(usedKeyCount))

	// Each user should have 9 failures and 1 success
	for user := range userSuccessCount {
		assert.Equal(t, 1, userSuccessCount[user])
		// All but 1 of the attempts to register beta keys should fail.
		assert.Equal(t, numOfBetaKeys-1, userFailureCount[user])
	}

	// Each beta key should only have been successfully claimed 1 time
	for _, num := range usedKeyCount {
		assert.Equal(t, 1, num)
	}
}
