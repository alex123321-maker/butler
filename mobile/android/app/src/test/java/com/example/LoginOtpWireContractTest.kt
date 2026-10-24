package com.example

import com.example.data.remote.LoginOtpRequest
import com.squareup.moshi.Moshi
import com.squareup.moshi.kotlin.reflect.KotlinJsonAdapterFactory
import org.junit.Assert.*
import org.junit.Test

class LoginOtpWireContractTest {
    @Test fun privateBodyUsesExactBackendFieldsAndRedactedFormatting() {
        val candidate = LoginOtpRequest("synthetic-event", "sms.app", "Example", "739284", 123L)
        val json = Moshi.Builder().add(KotlinJsonAdapterFactory()).build().adapter(LoginOtpRequest::class.java).toJson(candidate)
        assertTrue(json.contains("\"event_id\""))
        assertTrue(json.contains("\"package_name\""))
        assertTrue(json.contains("\"posted_at_epoch_ms\":123"))
        assertFalse(candidate.toString().contains("739284"))
    }
}
