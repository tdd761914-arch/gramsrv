import "@acton/build"
import "@acton/crypto"
import "@acton/emulation/network"
import "@acton/emulation/testing"
import "@acton/testing/expect"

import "@contracts/messages"
import "@contracts/types"
import "@wrappers/CustomFragmentCollection.gen"
import "@wrappers/CustomFragmentItem.gen"

fun setupCollection(): [CustomFragmentCollection, Treasury, KeyPair] {
    testing.setNow(1_800_000_000);
    val deployer = testing.treasury("customfragment deployer");
    val keyPair = crypto.seedToKeyPair(777);
    val collection = CustomFragmentCollection.fromStorage({
        mintedCount: 0,
        subwalletId: 0x4752414d,
        publicKey: keyPair.publicKey,
        collectionContent: createEmptyCell(),
        itemCode: build("CustomFragmentItem"),
        royalty: RoyaltyParams {
            numerator: 0,
            denominator: 1000,
            destination: deployer.address,
        }.toCell(),
    });
    collection.deploy(deployer.address, { value: ton("0.1") });
    return [collection, deployer, keyPair];
}

get fun `test customfragment: signed gift mint deploys a standard nft`() {
    val [collection, _, keyPair] = setupCollection();
    val owner = testing.treasury("gift owner");
    val authorization: MintAuthorization = {
        subwalletId: 0x4752414d,
        validSince: 1_799_999_990,
        validTill: 1_800_000_300,
        itemIndex: 42,
        ownerAddress: owner.address,
        content: beginCell().storeUint(1, 8).storeString("https://grams.example/gift/42.json").endCell(),
    };
    val signature = crypto.sign(keyPair.privateKey, authorization.toCell()) as bits512;

    val result = collection.sendMintNft(
        owner.address,
        signature,
        authorization,
        { value: ton("0.08"), bounce: true },
    );
    expect(result).toHaveSuccessfulTx<MintNft>({ from: owner.address, to: collection.address });

    val itemAddress = collection.getNftAddressByIndex(42);
    expect(result).toHaveSuccessfulDeploy({ from: collection.address, to: itemAddress });
    val data = CustomFragmentItem.fromAddress(itemAddress).getNftData();
    expect(data.initialized).toEqual(-1);
    expect(data.itemIndex).toEqual(42);
    expect(data.ownerAddress!).toEqual(owner.address);
    expect(data.collectionAddress).toEqual(collection.address);
}

get fun `test customfragment: authorization is bound to its wallet`() {
    val [collection, _, keyPair] = setupCollection();
    val owner = testing.treasury("authorized owner");
    val attacker = testing.treasury("other wallet");
    val authorization: MintAuthorization = {
        subwalletId: 0x4752414d,
        validSince: 1_799_999_990,
        validTill: 1_800_000_300,
        itemIndex: 77,
        ownerAddress: owner.address,
        content: createEmptyCell(),
    };
    val signature = crypto.sign(keyPair.privateKey, authorization.toCell()) as bits512;
    val result = collection.sendMintNft(
        attacker.address,
        signature,
        authorization,
        { value: ton("0.08"), bounce: true },
    );
    expect(result).toHaveFailedTx<MintNft>({
        from: attacker.address,
        to: collection.address,
        exitCode: 401,
    });
}
