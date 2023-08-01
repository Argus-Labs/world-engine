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

import {IERC20} from "../../../lib/IERC20.sol";
import {Cosmos} from "../CosmosTypes.sol";

/**
 * @dev Interface of the erc20 module's precompiled contract
 */
interface IERC20Module {
    ////////////////////////////////////////// EVENTS /////////////////////////////////////////////

    /**
     * @dev Emitted by the erc20 module when `amount` tokens are transferred from SDK coin (of
     * denomination `denom`) to an ERC20 token from `owner` to `recipient`.
     */
    event TransferCoinToErc20(
        string indexed denom, address indexed owner, address indexed recipient, Cosmos.Coin[] amount
    );

    /**
     * @dev Emitted by the erc20 module when `amount` tokens are transferred from ERC20 (of address
     * `token`) to an SDK coin from `owner` to `recipient`.
     */
    event TransferErc20ToCoin(
        address indexed token, address indexed owner, address indexed recipient, Cosmos.Coin[] amount
    );

    /////////////////////////////////////// READ METHODS //////////////////////////////////////////

    /**
     * @dev coinDenomForERC20Address returns the SDK coin denomination for the given ERC20 address.
     */
    function coinDenomForERC20Address(IERC20 token) external view returns (string memory);

    /**
     * @dev erc20AddressForCoinDenom returns the ERC20 address for the given SDK coin denomination.
     */
    function erc20AddressForCoinDenom(string calldata denom) external view returns (IERC20);

    ////////////////////////////////////// WRITE METHODS //////////////////////////////////////////

    /**
     * @dev transferCoinToERC20 transfers `amount` SDK coins to ERC20 tokens for `msg.sender`
     * @param denom the denomination of the SDK coin being transferred from
     * @param amount the amount of coins to transfer
     */
    function transferCoinToERC20(string calldata denom, uint256 amount) external returns (bool);

    /**
     * @dev transferCoinToERC20From transfers `amount` SDK coins to ERC20 tokens from `owner` to
     * `recipient`
     * @param denom the denomination of the SDK coin being transferred from
     * @param owner the address of the owner of the coins
     * @param recipient the address of the recipient of the tokens
     * @param amount the amount of coins to transfer
     */
    function transferCoinToERC20From(string calldata denom, address owner, address recipient, uint256 amount)
        external
        returns (bool);

    /**
     * @dev transferCoinToERC20To transfers `amount` SDK coins to ERC20 tokens from `msg.sender` to
     * `recipient`
     * @param denom the denomination of the SDK coin being transferred from
     * @param recipient the address of the recipient of the tokens
     * @param amount the amount of coins to transfer
     */
    function transferCoinToERC20To(string calldata denom, address recipient, uint256 amount) external returns (bool);

    /**
     * @dev transferERC20ToCoin transfers `amount` ERC20 tokens to SDK coins for `msg.sender`
     * @param token the ERC20 token being transferred from
     * @param amount the amount of tokens to transfer
     */
    function transferERC20ToCoin(IERC20 token, uint256 amount) external returns (bool);

    /**
     * @dev transferERC20ToCoinFrom transfers `amount` ERC20 tokens to SDK coins from `owner` to
     * `recipient`
     * @param token the ERC20 token being transferred from
     * @param owner the address of the owner of the coins
     * @param recipient the address of the recipient of the tokens
     * @param amount the amount of tokens to transfer
     */
    function transferERC20ToCoinFrom(IERC20 token, address owner, address recipient, uint256 amount)
        external
        returns (bool);

    /**
     * @dev transferERC20ToCoinTo transfers `amount` ERC20 tokens to SDK coins from `msg.sender` to
     * `recipient`
     * @param token the ERC20 token being transferred from
     * @param recipient the address of the recipient of the tokens
     * @param amount the amount of tokens to transfer
     */
    function transferERC20ToCoinTo(IERC20 token, address recipient, uint256 amount) external returns (bool);
}
