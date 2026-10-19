package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ContactEmailAddress 表示联系人邮箱地址。
type ContactEmailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// PhysicalAddress 表示联系人地址。
type PhysicalAddress struct {
	Street          string `json:"street,omitempty"`
	City            string `json:"city,omitempty"`
	State           string `json:"state,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
	CountryOrRegion string `json:"countryOrRegion,omitempty"`
}

// Contact 表示一个 Outlook 联系人。
type Contact struct {
	ID               string                `json:"id"`
	DisplayName      string                `json:"displayName"`
	EmailAddresses   []ContactEmailAddress `json:"emailAddresses"`
	CompanyName      string                `json:"companyName"`
	JobTitle         string                `json:"jobTitle"`
	MobilePhone      string                `json:"mobilePhone"`
	BusinessPhones   []string              `json:"businessPhones"`
	NickName         string                `json:"nickName,omitempty"`
	Birthday         string                `json:"birthday,omitempty"`
	PersonalNotes    string                `json:"personalNotes,omitempty"`
	Department       string                `json:"department,omitempty"`
	OfficeLocation   string                `json:"officeLocation,omitempty"`
	Profession       string                `json:"profession,omitempty"`
	BusinessHomePage string                `json:"businessHomePage,omitempty"`
	HomeAddress      *PhysicalAddress      `json:"homeAddress,omitempty"`
	BusinessAddress  *PhysicalAddress      `json:"businessAddress,omitempty"`
	OtherAddress     *PhysicalAddress      `json:"otherAddress,omitempty"`
}

// ListContacts 读取当前账号默认联系人文件夹中的联系人列表。
func ListContacts(ctx context.Context, accessToken string, top int) ([]Contact, error) {
	return ListContactsInFolder(ctx, accessToken, "", top)
}

// ListContactsInFolder 读取指定联系人文件夹中的联系人列表。
// folderID 为空时读取默认联系人文件夹。
func ListContactsInFolder(ctx context.Context, accessToken, folderID string, top int) ([]Contact, error) {
	if top <= 0 {
		top = 50
	}
	if top > 200 {
		top = 200
	}
	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,displayName,emailAddresses,companyName,jobTitle,mobilePhone,businessPhones,nickName,birthday,personalNotes,department,officeLocation,profession,businessHomePage,homeAddress,businessAddress,otherAddress")
	query.Set("$orderby", "displayName")
	endpoint := graphBaseURL + "/me/contacts?" + query.Encode()
	if strings.TrimSpace(folderID) != "" {
		endpoint = graphBaseURL + "/me/contactFolders/" + url.PathEscape(folderID) + "/contacts?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建联系人列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 联系人列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 联系人列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []Contact `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 联系人列表失败: %w", err)
	}
	return result.Value, nil
}

type ContactInput struct {
	DisplayName      string
	GivenName        string
	Surname          string
	EmailAddresses   []ContactEmailAddress
	CompanyName      string
	JobTitle         string
	MobilePhone      string
	BusinessPhones   []string
	NickName         string
	Birthday         string
	PersonalNotes    string
	Department       string
	OfficeLocation   string
	Profession       string
	BusinessHomePage string
	HomeAddress      *PhysicalAddress
	BusinessAddress  *PhysicalAddress
	OtherAddress     *PhysicalAddress
}

// buildContactPayload 构造 Graph 联系人请求体。
func buildContactPayload(input ContactInput) map[string]any {
	payload := map[string]any{
		"displayName": input.DisplayName,
	}
	if strings.TrimSpace(input.GivenName) != "" {
		payload["givenName"] = input.GivenName
	}
	if strings.TrimSpace(input.Surname) != "" {
		payload["surname"] = input.Surname
	}
	if len(input.EmailAddresses) > 0 {
		payload["emailAddresses"] = input.EmailAddresses
	}
	if strings.TrimSpace(input.CompanyName) != "" {
		payload["companyName"] = input.CompanyName
	}
	if strings.TrimSpace(input.JobTitle) != "" {
		payload["jobTitle"] = input.JobTitle
	}
	if strings.TrimSpace(input.MobilePhone) != "" {
		payload["mobilePhone"] = input.MobilePhone
	}
	if len(input.BusinessPhones) > 0 {
		payload["businessPhones"] = input.BusinessPhones
	}
	if strings.TrimSpace(input.NickName) != "" {
		payload["nickName"] = input.NickName
	}
	if strings.TrimSpace(input.Birthday) != "" {
		payload["birthday"] = input.Birthday
	}
	if strings.TrimSpace(input.PersonalNotes) != "" {
		payload["personalNotes"] = input.PersonalNotes
	}
	if strings.TrimSpace(input.Department) != "" {
		payload["department"] = input.Department
	}
	if strings.TrimSpace(input.OfficeLocation) != "" {
		payload["officeLocation"] = input.OfficeLocation
	}
	if strings.TrimSpace(input.Profession) != "" {
		payload["profession"] = input.Profession
	}
	if strings.TrimSpace(input.BusinessHomePage) != "" {
		payload["businessHomePage"] = input.BusinessHomePage
	}
	if input.HomeAddress != nil {
		payload["homeAddress"] = input.HomeAddress
	}
	if input.BusinessAddress != nil {
		payload["businessAddress"] = input.BusinessAddress
	}
	if input.OtherAddress != nil {
		payload["otherAddress"] = input.OtherAddress
	}
	return payload
}

// CreateContact 在默认联系人文件夹中创建联系人。
func CreateContact(ctx context.Context, accessToken string, input ContactInput) (*Contact, error) {
	return CreateContactInFolder(ctx, accessToken, "", input)
}

// CreateContactInFolder 在指定联系人文件夹中创建联系人。
// folderID 为空时创建到默认联系人文件夹。
func CreateContactInFolder(ctx context.Context, accessToken, folderID string, input ContactInput) (*Contact, error) {
	data, err := json.Marshal(buildContactPayload(input))
	if err != nil {
		return nil, fmt.Errorf("序列化创建联系人请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/contacts"
	if strings.TrimSpace(folderID) != "" {
		endpoint = graphBaseURL + "/me/contactFolders/" + url.PathEscape(folderID) + "/contacts"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("解析创建联系人响应失败: %w", err)
	}
	return &contact, nil
}

// UpdateContact 更新联系人。
func UpdateContact(ctx context.Context, accessToken, contactID string, input ContactInput) (*Contact, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return nil, fmt.Errorf("联系人 ID 不能为空")
	}
	data, err := json.Marshal(buildContactPayload(input))
	if err != nil {
		return nil, fmt.Errorf("序列化更新联系人请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建更新联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 更新联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 更新联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("解析更新联系人响应失败: %w", err)
	}
	return &contact, nil
}

// DeleteContact 删除联系人。
func DeleteContact(ctx context.Context, accessToken, contactID string) error {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return fmt.Errorf("联系人 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// SearchContacts 按显示名称前缀搜索联系人。
func SearchContacts(ctx context.Context, accessToken, query string, top int) ([]Contact, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("搜索关键字不能为空")
	}
	if top <= 0 {
		top = 20
	}
	if top > 200 {
		top = 200
	}

	escaped := strings.ReplaceAll(query, "'", "''")
	filter := fmt.Sprintf("startswith(displayName,'%s')", escaped)
	values := url.Values{}
	values.Set("$filter", filter)
	values.Set("$top", strconv.Itoa(top))
	values.Set("$select", "id,displayName,emailAddresses,companyName,jobTitle,mobilePhone,businessPhones,nickName,birthday,personalNotes,department,officeLocation,profession,businessHomePage,homeAddress,businessAddress,otherAddress")
	values.Set("$orderby", "displayName")
	endpoint := graphBaseURL + "/me/contacts?" + values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建联系人搜索请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 联系人搜索失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 联系人搜索返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []Contact `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 联系人搜索失败: %w", err)
	}
	return result.Value, nil
}

// ContactFolder 表示一个联系人文件夹。
type ContactFolder struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName"`
	ParentFolderID string `json:"parentFolderId"`
}

// ListContactFolders 读取联系人文件夹列表。
func ListContactFolders(ctx context.Context, accessToken string, top int) ([]ContactFolder, error) {
	if top <= 0 {
		top = 50
	}
	if top > 200 {
		top = 200
	}
	query := url.Values{}
	query.Set("$top", strconv.Itoa(top))
	query.Set("$select", "id,displayName,parentFolderId")
	query.Set("$orderby", "displayName")
	endpoint := graphBaseURL + "/me/contactFolders?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建联系人文件夹列表请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 联系人文件夹列表失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 联系人文件夹列表返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Value []ContactFolder `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 Graph 联系人文件夹列表失败: %w", err)
	}
	return result.Value, nil
}

// CreateContactFolder 创建联系人文件夹。
func CreateContactFolder(ctx context.Context, accessToken, displayName, parentID string) (*ContactFolder, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, fmt.Errorf("文件夹名称不能为空")
	}
	payload, err := json.Marshal(map[string]string{"displayName": displayName})
	if err != nil {
		return nil, fmt.Errorf("序列化创建联系人文件夹请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/contactFolders"
	if strings.TrimSpace(parentID) != "" {
		endpoint = graphBaseURL + "/me/contactFolders/" + url.PathEscape(parentID) + "/childFolders"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("创建联系人文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 创建联系人文件夹失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 创建联系人文件夹返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var folder ContactFolder
	if err := json.NewDecoder(resp.Body).Decode(&folder); err != nil {
		return nil, fmt.Errorf("解析创建联系人文件夹响应失败: %w", err)
	}
	return &folder, nil
}

// DeleteContactFolder 删除联系人文件夹。
func DeleteContactFolder(ctx context.Context, accessToken, folderID string) error {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" {
		return fmt.Errorf("联系人文件夹 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/contactFolders/" + url.PathEscape(folderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建删除联系人文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 删除联系人文件夹失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 删除联系人文件夹返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// ResolveContactFolderID 按显示名称查找联系人文件夹 ID。
func ResolveContactFolderID(ctx context.Context, accessToken, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("联系人文件夹名称不能为空")
	}
	folders, err := ListContactFolders(ctx, accessToken, 200)
	if err != nil {
		return "", err
	}
	for _, folder := range folders {
		if strings.EqualFold(folder.DisplayName, target) {
			return folder.ID, nil
		}
	}
	return target, nil
}

// GetContact 读取单个联系人的完整信息。
func GetContact(ctx context.Context, accessToken, contactID string) (*Contact, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return nil, fmt.Errorf("联系人 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("创建联系人详情请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 Graph 联系人详情失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Graph 联系人详情返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var contact Contact
	if err := json.NewDecoder(resp.Body).Decode(&contact); err != nil {
		return nil, fmt.Errorf("解析 Graph 联系人详情失败: %w", err)
	}
	return &contact, nil
}

// CopyContact 把联系人复制到指定联系人文件夹。
// newDisplayName 为空时沿用原显示名称。
func CopyContact(ctx context.Context, accessToken, contactID, destinationFolderID, newDisplayName string) (*Contact, error) {
	contact, err := GetContact(ctx, accessToken, contactID)
	if err != nil {
		return nil, err
	}
	input := ContactInput{
		DisplayName:      contact.DisplayName,
		CompanyName:      contact.CompanyName,
		JobTitle:         contact.JobTitle,
		MobilePhone:      contact.MobilePhone,
		EmailAddresses:   contact.EmailAddresses,
		BusinessPhones:   contact.BusinessPhones,
		NickName:         contact.NickName,
		Birthday:         contact.Birthday,
		PersonalNotes:    contact.PersonalNotes,
		Department:       contact.Department,
		OfficeLocation:   contact.OfficeLocation,
		Profession:       contact.Profession,
		BusinessHomePage: contact.BusinessHomePage,
		HomeAddress:      contact.HomeAddress,
		BusinessAddress:  contact.BusinessAddress,
		OtherAddress:     contact.OtherAddress,
	}
	if strings.TrimSpace(newDisplayName) != "" {
		input.DisplayName = strings.TrimSpace(newDisplayName)
	}
	return CreateContactInFolder(ctx, accessToken, destinationFolderID, input)
}

// MoveContact 把联系人移动到指定联系人文件夹。
// Graph 没有直接移动联系人的接口，因此这里使用复制到目标文件夹再删除原联系人的方式。
func MoveContact(ctx context.Context, accessToken, contactID, destinationFolderID string) (*Contact, error) {
	moved, err := CopyContact(ctx, accessToken, contactID, destinationFolderID, "")
	if err != nil {
		return nil, err
	}
	if err := DeleteContact(ctx, accessToken, contactID); err != nil {
		return nil, fmt.Errorf("联系人已复制到目标文件夹，但删除原联系人失败: %w", err)
	}
	return moved, nil
}

// RenameContactFolder 重命名联系人文件夹。
func RenameContactFolder(ctx context.Context, accessToken, folderID, displayName string) error {
	folderID = strings.TrimSpace(folderID)
	displayName = strings.TrimSpace(displayName)
	if folderID == "" {
		return fmt.Errorf("联系人文件夹 ID 不能为空")
	}
	if displayName == "" {
		return fmt.Errorf("文件夹名称不能为空")
	}
	payload, err := json.Marshal(map[string]string{"displayName": displayName})
	if err != nil {
		return fmt.Errorf("序列化重命名联系人文件夹请求失败: %w", err)
	}
	endpoint := graphBaseURL + "/me/contactFolders/" + url.PathEscape(folderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("创建重命名联系人文件夹请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 重命名联系人文件夹失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 重命名联系人文件夹返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// PermanentDeleteContact 永久删除联系人。
func PermanentDeleteContact(ctx context.Context, accessToken, contactID string) error {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return fmt.Errorf("联系人 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID) + "/permanentDelete"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("创建永久删除联系人请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 永久删除联系人失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 永久删除联系人返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// GetContactPhoto 读取联系人照片。
func GetContactPhoto(ctx context.Context, accessToken, contactID string) ([]byte, string, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return nil, "", fmt.Errorf("联系人 ID 不能为空")
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID) + "/photo/$value"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", fmt.Errorf("创建联系人照片请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("调用 Graph 联系人照片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("Graph 联系人照片返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取联系人照片失败: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// SetContactPhoto 设置联系人照片。
func SetContactPhoto(ctx context.Context, accessToken, contactID string, data []byte, contentType string) error {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return fmt.Errorf("联系人 ID 不能为空")
	}
	if len(data) == 0 {
		return fmt.Errorf("联系人照片内容不能为空")
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "image/jpeg"
	}
	endpoint := graphBaseURL + "/me/contacts/" + url.PathEscape(contactID) + "/photo/$value"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("创建更新联系人照片请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("调用 Graph 更新联系人照片失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("Graph 更新联系人照片返回异常状态: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
