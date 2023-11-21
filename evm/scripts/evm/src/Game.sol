// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

import {IRouter} from "./router.sol";

contract Game {
    IRouter internal router;
    string internal Namespace = "TESTGAME";

    struct Join {
        bool ok;
    }

    struct JoinResult {
        bool Success;
    }

    uint64 internal JoinID = 3;

    struct Move {
        string Direction;
    }

    struct MoveResult {
        int64 X;
        int64 Y;
    }

    uint64 internal MoveID = 4;

    struct QueryLocation {
        string ID;
    }

    struct QueryLocationResponse {
        int64 X;
        int64 Y;
    }

    string internal queryLocationName = "location";

    constructor() {
        router = IRouter(0x356833c4666fFB6bFccbF8D600fa7282290dE073);
    }

    function joinGame() public returns (bool) {
        Join memory joinMsg = Join(true);
        bytes memory encoded = abi.encode(joinMsg);
        bool ok = router.sendMessage(encoded, JoinID, Namespace);
        if (!ok) {
            revert("router couldn't send the message");
        }
        return true;
    }

    function getJoinResult(string calldata txHash) public returns (JoinResult memory, string memory, uint32) {
        (bytes memory txResult, string memory errMsg, uint32 code) =  router.messageResult(txHash);
         JoinResult memory res = abi.decode(txResult, (JoinResult));
        return (res, errMsg, code);
    }

    function movePlayer(string calldata direction) public returns (bool) {
        Move memory moveMsg = Move(direction);
        bytes memory encoded = abi.encode(moveMsg);
        bool ok = router.sendMessage(encoded, MoveID, Namespace);
        if (!ok) {
            revert("router couldn't send the message");
        }
        return true;
    }

    function getMoveResult(string memory txHash) public returns (MoveResult memory, string memory, uint32) {
        (bytes memory txResult, string memory errMsg, uint32 code) =  router.messageResult(txHash);
        MoveResult memory res = abi.decode(txResult, (MoveResult));
        return (res, errMsg, code);
    }

    function Location(string memory name) public returns (int64, int64) {
        QueryLocation memory q = QueryLocation(name);
        bytes memory queryBz = abi.encode(q);
        bytes memory bz = router.query(queryBz, queryLocationName, Namespace);
        QueryLocationResponse memory res = abi.decode(bz, (QueryLocationResponse));
        return (res.X, res.Y);
    }
}