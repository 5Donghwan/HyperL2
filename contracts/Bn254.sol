// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

library Bn254 {
    uint256 internal constant BASE_FIELD_MODULUS =
        21888242871839275222246405745257275088696311157297823662689037894645226208583;
    uint256 internal constant SCALAR_FIELD_MODULUS =
        21888242871839275222246405745257275088548364400416034343698204186575808495617;

    struct G1Point {
        uint256 x;
        uint256 y;
    }

    struct G2Point {
        uint256[2] x;
        uint256[2] y;
    }

    function isInfinity(G1Point memory point) internal pure returns (bool) {
        return point.x == 0 && point.y == 0;
    }

    function negate(G1Point memory point) internal pure returns (G1Point memory) {
        if (isInfinity(point)) {
            return G1Point(0, 0);
        }
        return G1Point(point.x, BASE_FIELD_MODULUS - (point.y % BASE_FIELD_MODULUS));
    }

    function g1Add(
        G1Point memory lhs,
        G1Point memory rhs
    ) internal view returns (G1Point memory result) {
        uint256[4] memory input = [lhs.x, lhs.y, rhs.x, rhs.y];
        bool success;
        assembly {
            success := staticcall(gas(), 0x06, input, 0x80, result, 0x40)
        }
        require(success, "bn254 add failed");
    }

    function g1Mul(
        G1Point memory point,
        uint256 scalar
    ) internal view returns (G1Point memory result) {
        uint256[3] memory input = [point.x, point.y, scalar];
        bool success;
        assembly {
            success := staticcall(gas(), 0x07, input, 0x60, result, 0x40)
        }
        require(success, "bn254 mul failed");
    }

    function pairing(
        G1Point[] memory p1,
        G2Point[] memory p2
    ) internal view returns (bool) {
        require(p1.length == p2.length, "pairing length mismatch");

        uint256 elements = p1.length;
        uint256 inputSize = elements * 6;
        uint256[] memory input = new uint256[](inputSize);

        for (uint256 i = 0; i < elements; ++i) {
            uint256 offset = i * 6;
            input[offset] = p1[i].x;
            input[offset + 1] = p1[i].y;
            input[offset + 2] = p2[i].x[0];
            input[offset + 3] = p2[i].x[1];
            input[offset + 4] = p2[i].y[0];
            input[offset + 5] = p2[i].y[1];
        }

        uint256[1] memory output;
        bool success;
        assembly {
            success := staticcall(gas(), 0x08, add(input, 0x20), mul(inputSize, 0x20), output, 0x20)
        }
        require(success, "bn254 pairing failed");
        return output[0] == 1;
    }

    function pairingProd4(
        G1Point memory a1,
        G2Point memory a2,
        G1Point memory b1,
        G2Point memory b2,
        G1Point memory c1,
        G2Point memory c2,
        G1Point memory d1,
        G2Point memory d2
    ) internal view returns (bool) {
        G1Point[] memory p1 = new G1Point[](4);
        G2Point[] memory p2 = new G2Point[](4);
        p1[0] = a1;
        p1[1] = b1;
        p1[2] = c1;
        p1[3] = d1;
        p2[0] = a2;
        p2[1] = b2;
        p2[2] = c2;
        p2[3] = d2;
        return pairing(p1, p2);
    }
}
