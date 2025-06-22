export enum AsyncValueStatus {
	initial = "initial",
	pending = "pending",
	resolved = "resolved",
	failedToResolv = "failedToResolve"
}

export interface AsyncValue <T> {
	readonly status: AsyncValueStatus;
	readonly value: T | null;
}
