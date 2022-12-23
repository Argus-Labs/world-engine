// SPDX-License-Identifier: UNLICENSED
pragma solidity ^0.8.13;

contract Quest {

    event QuestComplete(address);

    mapping(address => bool) internal completed;

    function completeQuest(address a) public {
        completed[a] = true;
        emit QuestComplete(a);
    }

}
