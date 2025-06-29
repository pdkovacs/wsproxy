import type { AxiosError } from "axios";
import axios from "axios";

export const postMsg = async (): Promise<{result: string}> => {
	return postMsgToSomeone(null);
};

export const postMsgToSomeone = async (whom: string|null, what: string = "hello"): Promise<{result: string}> => {
	try {
		const response = await axios({
			method: "POST",
			url: "/api/message",
			data: {
				whom,
				what
			}
		});
		return {result: response.data.whom};
	} catch (err) {
		const axiosError = err as AxiosError;
		throw new Error(`${axiosError.response?.status} ${axiosError.response?.statusText}`);
	}
};
