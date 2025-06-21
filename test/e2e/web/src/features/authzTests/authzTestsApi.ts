import type { AxiosError } from "axios";
import axios from "axios";
import { isNil } from "lodash-es";

export const postHello = async (): Promise<{result: string}> => {
	return postHelloToSomeone(null);
};

export const postHelloToSomeone = async (whom: string|null): Promise<{result: string}> => {
	try {
		const response = await axios({
			method: "POST",
			url: "/api/hello",
			data: isNil(whom)
				? undefined
				: {
					whom
				}
		});
		return {result: response.data.whom};
	} catch (err) {
		const axiosError = err as AxiosError;
		throw new Error(`${axiosError.response?.status} ${axiosError.response?.statusText}`);
	}
};
