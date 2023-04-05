// SPDX-License-Identifier: MIT
//
// Copyright (c) 2023 Berachain Foundation
//
// Permission is hereby granted, free of charge, to any person
// obtaining a copy of this software and associated documentation
// files (the "Software"), to deal in the Software without
// restriction, including without limitation the rights to use,
// copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following
// conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
// OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
// HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
// WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

pragma solidity ^0.8.4;

import "../../../../lib/forge-std/src/Script.sol";
import "../../../../lib/forge-std/src/console2.sol";
import "../staking.sol";
import "./LiquidStaking.sol";

contract Deploy is Script {
    address precompile = address(0xd9A998CaC66092748FfEc7cFBD155Aae1737C2fF);

    // TODO: script is broken because it runs its own evm; need Foundry fix.

    function run() public {
        vm.startBroadcast();

        // Calling the Liquid Staking Contract, which calls the staking precompile.
        LiquidStaking ls = new LiquidStaking(
            "hello",
            "sss",
            precompile,
            address(0x7F04B06a9C507B366567B09E82C4bC037e87d0e6)
        );

        // Low-level call.
        // (bool success, bytes memory data) = address(ls).staticcall(
        //     abi.encodeWithSignature("getActiveValidators()")
        // );
        // console2.logBool(success);
        // console2.logBytes(data);
        // require(success, "Failed to get active validators from the call");
        // address[] memory vals = abi.decode(data, (address[]));

        // High-level call.
        // address[] memory vals = ls.getActiveValidators();

        // Calling the staking precompile contract directly.
        IStakingModule staking = IStakingModule(precompile);

        // Low-level call.
        // (bool success, bytes memory data) = address(staking).staticcall(
        //     abi.encodeWithSignature("getActiveValidators()")
        // );
        // require(success, "Failed to get active validators from the call");
        // console2.logBytes(data);
        // address[] memory vals = abi.decode(data, (address[]));

        // High-level call.
        // address[] memory vals = staking.getActiveValidators();

        vm.stopBroadcast();
    }
}
