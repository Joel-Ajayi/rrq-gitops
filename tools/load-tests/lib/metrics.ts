import { Counter, Trend, Rate } from 'k6/metrics';

export const TransferSuccessCounter = new Counter('transfers_success_total');
export const TransferFailureCounter = new Counter('transfers_failure_total');
export const TransferDurationTrend = new Trend('transfer_duration_ms');
export const TransferErrorRate = new Rate('transfer_error_rate');

export const DepositSuccessCounter = new Counter('deposits_success_total');
export const DepositDurationTrend = new Trend('deposit_duration_ms');

export const BalanceDurationTrend = new Trend('balance_duration_ms');
