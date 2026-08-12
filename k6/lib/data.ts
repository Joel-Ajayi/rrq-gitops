import { SharedArray } from 'k6/data';

export interface SeedData {
  jwts: string[];
  wallets: string[][];
}


export interface WalletPair {
  fromWallet: string;
  toWallet: string;
  jwt: string;
}

export interface DepositContext {
  walletId: string;
  jwt: string;
}

const seedData = new SharedArray<SeedData>('test-data', function () {
  return [JSON.parse(open('../test-data.json'))];
})[0];

/**
 * Selects a real wallet pair from test-data.json generated directly from API endpoints.
 */
export function selectWalletPair(vu: number, iter: number): WalletPair {
  const merchantCount = seedData.jwts.length;
  const merchantIdx = (vu - 1) % merchantCount;
  const toMerchantIdx = vu % merchantCount;

  const fromWallets = seedData.wallets[merchantIdx];
  const toWallets = seedData.wallets[toMerchantIdx];

  const fromWalletIdx = (iter + vu * 37) % fromWallets.length;
  const toWalletIdx = (iter + vu * 43) % toWallets.length;

  return {
    fromWallet: fromWallets[fromWalletIdx],
    toWallet: toWallets[toWalletIdx],
    jwt: seedData.jwts[merchantIdx],
  };
}

/**
 * Selects a real wallet from test-data.json for deposit testing.
 */
export function selectDepositWallet(vu: number, iter: number): DepositContext {
  const merchantCount = seedData.jwts.length;
  const merchantIdx = (vu - 1) % merchantCount;

  const wallets = seedData.wallets[merchantIdx];
  const walletIdx = (iter + vu * 37) % wallets.length;

  return {
    walletId: wallets[walletIdx],
    jwt: seedData.jwts[merchantIdx],
  };
}
