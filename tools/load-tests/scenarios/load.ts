import { Options, RampingArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, selectDepositWallet, WalletPair, DepositContext } from '../lib/data.ts';
import { submitTransfer, submitDeposit, getWalletBalance, getActiveJwt, refreshToken } from '../lib/api.ts';

import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    slo_load_test: CONFIG.workloads.load as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.load,
};

export default function (): void {
  const rand: number = Math.random();
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;

  if (rand < 0.20) {
    // 20% Wallet Deposits
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitDeposit(walletId, 500, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.40) {
    // 20% Wallet Balances (Reads)
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = getWalletBalance(walletId, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else {
    // 60% Transfers
    const { fromWallet, toWallet, jwt, apiKey, merchantIdx }: WalletPair = selectWalletPair(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitTransfer(fromWallet, toWallet, 100, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  }
}
