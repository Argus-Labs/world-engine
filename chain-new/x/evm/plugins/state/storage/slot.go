// SPDX-License-Identifier: BUSL-1.1
//
// Copyright (C) 2023, Berachain Foundation. All rights reserved.
// Use of this software is govered by the Business Source License included
// in the LICENSE file of this repository and at www.mariadb.com/bsl11.
//
// ANY USE OF THE LICENSED WORK IN VIOLATION OF THIS LICENSE WILL AUTOMATICALLY
// TERMINATE YOUR RIGHTS UNDER THIS LICENSE FOR THE CURRENT AND ALL OTHER
// VERSIONS OF THE LICENSED WORK.
//
// THIS LICENSE DOES NOT GRANT YOU ANY RIGHT IN ANY TRADEMARK OR LOGO OF
// LICENSOR OR ITS AFFILIATES (PROVIDED THAT YOU MAY USE A TRADEMARK OR LOGO OF
// LICENSOR AS EXPRESSLY REQUIRED BY THIS LICENSE).
//
// TO THE EXTENT PERMITTED BY APPLICABLE LAW, THE LICENSED WORK IS PROVIDED ON
// AN “AS IS” BASIS. LICENSOR HEREBY DISCLAIMS ALL WARRANTIES AND CONDITIONS,
// EXPRESS OR IMPLIED, INCLUDING (WITHOUT LIMITATION) WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, NON-INFRINGEMENT, AND
// TITLE.

package storage

import (
	"fmt"
	"strings"

	"pkg.berachain.dev/polaris/eth/common"
	"pkg.berachain.dev/polaris/lib/errors"
	libtypes "pkg.berachain.dev/polaris/lib/types"
)

// Compile-time interface assertions.
var (
	_ libtypes.Cloneable[*Slot] = (*Slot)(nil)
	_ fmt.Stringer              = (*Slot)(nil)
)

// NewSlot creates a new State instance.
func NewSlot(key, value common.Hash) *Slot {
	return &Slot{
		Key:   key.Hex(),
		Value: value.Hex(),
	}
}

// ValidateBasic checks to make sure the key is not empty.
func (s *Slot) ValidateBasic() error {
	if strings.TrimSpace(s.Key) == "" {
		return errors.Wrapf(ErrInvalidState, "key cannot be empty %s", s.Key)
	}

	return nil
}

// Clone implements `types.Cloneable`.
func (s *Slot) Clone() *Slot {
	return &Slot{
		Key:   s.Key,
		Value: s.Value,
	}
}
